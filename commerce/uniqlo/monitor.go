package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
)

// State has no runtime configuration; source observations belong to checkpoints.
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

var proxies = monitord.RequiredSecret("proxies", "uniqlo")

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "uniqlo",
		Description: "Uniqlo online restocks for selected color and size pairs",
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
	identity := struct {
		Country    string
		Language   string
		Product    string
		PriceGroup string
		Variants   []selection
	}{settings.Country, settings.Language, settings.Product, settings.PriceGroup, slices.Clone(settings.Variants)}
	slices.SortFunc(identity.Variants, func(a, b selection) int { return strings.Compare(a.key(), b.key()) })
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
	key := "online:" + m.scope
	var previous snapshot
	if _, err := session.Checkpoint(key, &previous); err != nil {
		return err
	}
	if time.Now().Before(previous.NextAttempt) {
		return fmt.Errorf("uniqlo backoff until %s", previous.NextAttempt.UTC().Format(time.RFC3339))
	}
	available, retry, err := m.fetch(ctx)
	if err != nil {
		previous.Failures = min(previous.Failures+1, 5)
		previous.NextAttempt = time.Now().Add(max(settings.Interval, time.Minute*time.Duration(1<<previous.Failures), retry))
		if commitErr := session.Commit(ctx, func(tx *monitord.Tx[State]) error {
			return tx.Checkpoint(key, previous)
		}); commitErr != nil {
			return fmt.Errorf("save backoff: %w", commitErr)
		}

		return err
	}
	next := snapshot{
		Available: available,
		Sequence:  previous.Sequence,
	}

	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		for _, variant := range settings.Variants {
			wasAvailable, known := previous.Available[variant.key()]
			if !known || wasAvailable || !available[variant.key()] {
				continue
			}
			next.Sequence++
			event := monitord.Event{
				ID:          fmt.Sprintf("uniqlo:%s:%d", m.scope, next.Sequence),
				Title:       "Uniqlo online restock",
				Description: fmt.Sprintf("%s · color %s · size %s", settings.Product, variant.Color, variant.Size),
				URL:         productURL(variant),
				Color:       0xFF0000,
				Footer:      "Online stock observation • Not a reservation",
			}
			if err := tx.Emit(event); err != nil {
				return err
			}
		}

		return tx.Checkpoint(key, next)
	})
}
