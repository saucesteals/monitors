package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
)

type State struct{}
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
	token  string
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "youtube", Description: "New uploads from any public YouTube channel"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(45*time.Second))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	if err := validate(); err != nil {
		return err
	}

	m.client = &http.Client{Timeout: 20 * time.Second, Transport: http.DefaultTransport.(*http.Transport).Clone(), CheckRedirect: func(req *http.Request, via []*http.Request) error {
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
	return nil
}

func (m *monitor) Stop(context.Context) error {
	if m.client != nil {
		m.client.CloseIdleConnections()
	}
	return nil
}

func (m *monitor) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "monitord-youtube")
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("source request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("source HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(b) > 8<<20 {
		return nil, fmt.Errorf("source response unreadable or oversized")
	}
	return b, nil
}

func (m *monitor) check(ctx context.Context, s *monitord.Session[State]) error {
	items, err := m.fetch(ctx)
	if err != nil {
		return err
	}
	// The endpoint/filter fingerprint keeps independently configured watches apart.
	raw, _ := json.Marshal(sourceIdentity())
	scope := fmt.Sprintf("%x", sha256.Sum256(raw))[:20]
	key := "source:" + scope
	var cursor Cursor
	found, err := s.Checkpoint(key, &cursor)
	if err != nil {
		return err
	}
	if cursor.Seen == nil {
		cursor.Seen = map[string]bool{}
	}
	if !found {
		for _, item := range items {
			cursor.Seen[item.ID] = true
		}
		return s.Commit(ctx, func(tx *monitord.Tx[State]) error { return tx.Checkpoint(key, cursor) })
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
				event.ID = "youtube:" + scope + ":" + item.ID
				events = append(events, event)
			}
		}
		if err = s.Commit(ctx, func(tx *monitord.Tx[State]) error {
			for _, event := range events {
				if err := tx.Emit(event); err != nil {
					return err
				}
			}
			return tx.Checkpoint(key, next)
		}); err != nil {
			return err
		}
		cursor = next
	}
	return nil
}

func clip(value string, n int) string {
	r := []rune(value)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return value
}

func matches(value, filter string) bool {
	return filter == "" || strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}
