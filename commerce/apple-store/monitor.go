package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
)

// State holds no user-editable runtime state; observations live in checkpoints.
type State struct{}

type snapshot struct {
	Available   map[string]bool `json:"available"`
	Sequence    uint64          `json:"sequence"`
	Failures    int             `json:"failures"`
	NextAttempt time.Time       `json:"next_attempt,omitempty"`
}

type transport interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

type monitor struct {
	client transport
	scope  string
}

var proxies = monitord.RequiredSecret("proxies", "apple-store")

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "apple-store",
		Description: "Apple Store pickup restocks by part number and location",
	}
}

func (m *monitor) Plan() monitord.Plan[State] {
	if settings.UseProxy {
		return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(30*time.Second), monitord.WithSecrets(proxies))
	}

	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(30*time.Second))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	if err := validate(); err != nil {
		return err
	}
	// Presentation, cadence and transport changes must not reset the baseline.
	identity := struct {
		Storefront string
		Location   string
		Stores     []string
		Parts      []string
	}{settings.Storefront, settings.Location, slices.Clone(settings.StoreIDs), slices.Clone(settings.Parts)}
	slices.Sort(identity.Stores)
	slices.Sort(identity.Parts)
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	m.scope = fmt.Sprintf("%x", sha256.Sum256(raw))[:20]
	if settings.UseProxy {
		m.client, err = httpx.NewProxyClient(env.Secrets(), proxies)
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

func (m *monitor) check(ctx context.Context, session *monitord.Session[State]) error {
	key := "pickup:" + m.scope
	var previous snapshot
	if _, err := session.Checkpoint(key, &previous); err != nil {
		return err
	}
	if time.Now().Before(previous.NextAttempt) {
		return fmt.Errorf("apple backoff until %s", previous.NextAttempt.UTC().Format(time.RFC3339))
	}
	stores, retry, err := m.fetch(ctx)
	if err != nil {
		previous.Failures = min(previous.Failures+1, 5)
		delay := max(settings.Interval, time.Minute*time.Duration(1<<previous.Failures), retry)
		previous.NextAttempt = time.Now().Add(delay)
		if commitErr := session.Commit(ctx, func(tx *monitord.Tx[State]) error {
			return tx.Checkpoint(key, previous)
		}); commitErr != nil {
			return fmt.Errorf("save backoff: %w", commitErr)
		}

		return err
	}

	next := snapshot{
		Available: make(map[string]bool),
		Sequence:  previous.Sequence,
	}
	var events []monitord.Event
	for _, store := range stores {
		for _, part := range settings.Parts {
			product := store.Parts[part]
			id := store.Number + ":" + part
			available := product.PickupDisplay == "available"
			next.Available[id] = available
			wasAvailable, known := previous.Available[id]
			// A new or reappearing store/part starts silently, never as a restock.
			if known && !wasAvailable && available {
				next.Sequence++
				event := pickupEvent(store, part)
				event.ID = fmt.Sprintf("pickup:%s:%d", m.scope, next.Sequence)
				events = append(events, event)
			}
		}
	}
	if len(events) > 128 {
		return fmt.Errorf("more than 128 restocks; narrow parts or stores")
	}

	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		for _, event := range events {
			if err := tx.Emit(event); err != nil {
				return err
			}
		}

		return tx.Checkpoint(key, next)
	})
}
