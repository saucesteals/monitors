package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
	"github.com/saucesteals/shop/provider/facebook"
)

type State struct {
	Scope string          `json:"scope"`
	Seen  map[string]bool `json:"seen"`
}
type Cursor struct {
	Seen map[string]bool `json:"seen"`
}

type Observation struct {
	ID    string
	Event monitord.Event
	Match bool
}

type monitor struct {
	client *http.Client
	pool   *httpx.ProxyClient
	store  *facebook.Store
}

var proxies = monitord.RequiredSecret("proxies", "isps")

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "facebook-burlington-wood-new", Description: "TJ's wood listings within 20 miles of Burlington, Connecticut"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(4*time.Minute), monitord.WithSecrets(proxies))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	if err := validate(); err != nil {
		return err
	}

	var err error
	m.pool, err = httpx.NewProxyClient(env.Secrets(), proxies)
	if err != nil {
		return err
	}
	m.client = &http.Client{Timeout: 20 * time.Second, Transport: &burlingtonTransport{base: m.pool}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("HTTPS redirect required")
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			req.Header.Del("Authorization")
		}
		return nil
	}}
	m.store, err = facebook.New(m.client)
	if err != nil {
		m.pool.CloseIdleConnections()
		return err
	}
	return nil
}

func (m *monitor) Stop(context.Context) error {
	if m.client != nil {
		m.client.CloseIdleConnections()
	}
	return nil
}

func (m *monitor) check(ctx context.Context, s *monitord.Session[State]) error {
	items, err := m.fetch(ctx)
	if err != nil {
		return err
	}
	// The endpoint/filter fingerprint keeps independently configured watches apart.
	matched := 0
	for _, item := range items {
		if item.Match {
			matched++
		}
	}
	slog.InfoContext(ctx, "marketplace scan complete", "listings", len(items), "matching", matched)
	raw, _ := json.Marshal(sourceIdentity())
	scope := fmt.Sprintf("%x", sha256.Sum256(raw))[:20]
	state := s.State()
	cursor := Cursor{Seen: state.Seen}
	if cursor.Seen == nil {
		// One-time migration of this deployment's original checkpoint. Its
		// observations survive the move to operator-editable dedupe state.
		if _, err := s.Checkpoint("source:9783fb1da603646ddc83", &cursor); err != nil {
			return err
		}
		if cursor.Seen == nil {
			cursor.Seen = map[string]bool{}
		}
	}
	if state.Scope != scope {
		for _, item := range items {
			cursor.Seen[item.ID] = true
		}
		return s.Commit(ctx, func(tx *monitord.Tx[State]) error {
			*tx.State = State{Scope: scope, Seen: cursor.Seen}
			return nil
		})
	}
	// Bounded commits let large feeds progress without exceeding the outbox cap.
	for start := 0; start < len(items); start += 128 {
		end := min(start+128, len(items))
		next := Cursor{Seen: make(map[string]bool, len(cursor.Seen)+end-start)}
		for id, value := range cursor.Seen {
			next.Seen[id] = value
		}
		var events []monitord.Event
		for _, item := range items[start:end] {
			if next.Seen[item.ID] {
				continue
			}
			next.Seen[item.ID] = true
			if item.Match {
				event := item.Event
				event.ID = "marketplace:" + scope + ":" + item.ID
				events = append(events, event)
			}
		}
		if err := s.Commit(ctx, func(tx *monitord.Tx[State]) error {
			for _, event := range events {
				if err := tx.Emit(event); err != nil {
					return err
				}
			}
			*tx.State = State{Scope: scope, Seen: next.Seen}
			return nil
		}); err != nil {
			return err
		}
		cursor = next
	}
	return nil
}
