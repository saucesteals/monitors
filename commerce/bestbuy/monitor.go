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

// State has no editable runtime fields; source observations use checkpoints.
type State struct{}

type checkpoint struct {
	Quantities  map[string]int `json:"quantities"`
	Sequence    uint64         `json:"sequence"`
	Failures    int            `json:"failures"`
	NextAttempt time.Time      `json:"next_attempt,omitempty"`
}

type transport interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

type monitor struct {
	client transport
	scope  string
}

var proxies = monitord.RequiredSecret("proxies", "bestbuy")

func main() { monitord.Run(&monitor{}) }

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "bestbuy",
		Description: "Best Buy local pickup restocks and optional quantity changes",
	}
}

func (m *monitor) Plan() monitord.Plan[State] {
	if settings.UseProxy {
		return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(45*time.Second), monitord.WithSecrets(proxies))
	}

	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(45*time.Second))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	if err := validate(); err != nil {
		return err
	}
	identity := struct {
		SKU    string
		ZIP    string
		Radius float64
		Stores []string
	}{
		SKU:    settings.SKU,
		ZIP:    settings.ZIPCode,
		Radius: settings.RadiusMiles,
		Stores: slices.Clone(settings.StoreIDs),
	}
	slices.Sort(identity.Stores)
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
	var previous checkpoint
	if _, err := session.Checkpoint(key, &previous); err != nil {
		return err
	}
	if time.Now().Before(previous.NextAttempt) {
		return fmt.Errorf("best buy backoff until %s", previous.NextAttempt.UTC().Format(time.RFC3339))
	}
	stocks, retry, err := m.fetch(ctx)
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

	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		next := checkpoint{
			Quantities: make(map[string]int, len(stocks)),
			Sequence:   previous.Sequence,
		}
		for _, stock := range stocks {
			next.Quantities[stock.ID] = stock.Quantity
			old, known := previous.Quantities[stock.ID]
			if !known || old == stock.Quantity || (!settings.NotifyQuantityChanges && !(old == 0 && stock.Quantity > 0)) {
				continue
			}
			next.Sequence++
			event := stockEvent(stock, old)
			event.ID = fmt.Sprintf("bestbuy:%s:%d", m.scope, next.Sequence)
			if err := tx.Emit(event); err != nil {
				return err
			}
		}

		return tx.Checkpoint(key, next)
	})
}
