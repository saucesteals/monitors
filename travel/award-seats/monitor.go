package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
	"regexp"
	"sort"
	"time"
)

var proxySecret = monitord.RequiredSecret("proxies", "award-seats")

type State struct{}
type Snapshot struct {
	Offers   map[string]Offer
	Sequence uint64
}

type transport interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

type monitor struct{ client transport }

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "award-seats", Description: "Premium-cabin Alaska Atmos partner awards"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	if settings.UseProxy {
		return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(2*time.Minute), monitord.WithSecrets(proxySecret))
	}
	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(2*time.Minute))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	airport := regexp.MustCompile(`^[A-Z]{3}$`)
	if !airport.MatchString(settings.Origin) || !airport.MatchString(settings.Destination) || settings.Origin == settings.Destination || settings.Passengers < 1 || settings.Passengers > 9 || settings.MaxPoints < 1 || settings.MaxStops < 0 || settings.MaxStops > 2 || len(settings.Dates) == 0 || len(settings.Dates) > 14 || len(settings.Cabins) == 0 || settings.Interval < time.Minute {
		return fmt.Errorf("invalid award-search configuration")
	}
	for _, date := range settings.Dates {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return err
		}
	}
	var err error
	if settings.UseProxy {
		m.client, err = httpx.NewProxyClient(env.Secrets(), proxySecret)
	} else {
		m.client, err = httpx.NewClient()
	}
	return err
}

func (m *monitor) Stop(context.Context) error {
	if m.client != nil {
		m.client.CloseIdleConnections()
	}
	return nil
}

func (m *monitor) check(ctx context.Context, s *monitord.Session[State]) error {
	raw, _ := json.Marshal(sourceIdentity())
	scope := fmt.Sprintf("%x", sha256.Sum256(raw))[:20]
	for _, date := range settings.Dates {
		if date < time.Now().UTC().Format("2006-01-02") {
			continue
		}
		offers, err := m.search(ctx, date)
		if err != nil {
			return err
		}
		key := "awards:" + scope + ":" + date
		var old Snapshot
		found, err := s.Checkpoint(key, &old)
		if err != nil {
			return err
		}
		next := Snapshot{Offers: offers, Sequence: old.Sequence}
		ids := make([]string, 0, len(offers))
		for id := range offers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var events []monitord.Event
		if found {
			for _, id := range ids {
				o := offers[id]
				previous, exists := old.Offers[id]
				if !exists || o.Points < previous.Points || o.Cash < previous.Cash || o.Seats > previous.Seats {
					next.Sequence++
					e := awardEvent(o)
					e.ID = fmt.Sprintf("award:%s:%s:%d", scope, date, next.Sequence)
					events = append(events, e)
				}
			}
		}
		// Never silently discard a large change set or advance its checkpoint.
		if len(events) > 128 {
			return fmt.Errorf("more than 128 award changes; narrow the search")
		}
		if err := s.Commit(ctx, func(tx *monitord.Tx[State]) error {
			for _, e := range events {
				if err := tx.Emit(e); err != nil {
					return err
				}
			}
			return tx.Checkpoint(key, next)
		}); err != nil {
			return err
		}
	}
	return nil
}
