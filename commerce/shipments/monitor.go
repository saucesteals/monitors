package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
)

// State is empty: source observations and queue positions belong in checkpoints.
type State struct{}

type cursor struct {
	After string `json:"after"`
}

type monitor struct {
	service   *tracking.Service
	transport *http.Transport
	filter    tracking.Filter
	scope     string
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "shipments",
		Description: "Saved packages: scan changes and meaningful delivery estimates",
	}
}

func (m *monitor) Plan() monitord.Plan[State] {
	// Each lookup has its own deadline; leave time for local reads and commits.
	timeout := settings.LookupTimeout*time.Duration(settings.MaxRefreshes) + time.Minute

	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(timeout))
}

func (m *monitor) Start(_ context.Context, _ monitord.Environment) error {
	filter, err := validate()
	if err != nil {
		return err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	client, err := shop.New(shop.Options{
		ConfigDir: settings.ShopConfig,
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   settings.LookupTimeout,
		},
	})
	if err != nil {
		transport.CloseIdleConnections()

		return fmt.Errorf("open Shop configuration: %w", err)
	}
	m.service = client.Tracking()
	m.transport = transport
	m.filter = filter
	m.scope = digest(client.ConfigDir())

	return nil
}

func (m *monitor) Stop(context.Context) error {
	if m.transport != nil {
		m.transport.CloseIdleConnections()
	}

	return nil
}

func (m *monitor) check(ctx context.Context, session *monitord.Session[State]) error {
	// Shop has no paginated listing. Read once; bound both kinds of processing
	// below. An independent observation queue keeps old deliveries visible
	// without making the live refresh queue wait behind an archive.
	entries, err := m.service.List(ctx, tracking.Filter{All: true})
	if err != nil {
		return fmt.Errorf("list saved shipments: %w", err)
	}
	entries = slices.DeleteFunc(entries, func(entry tracking.Shipment) bool { return !selected(entry) })
	active := m.filter.Select(entries, time.Now())
	batch, err := m.batch(session, "refresh", active, settings.MaxRefreshes)
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range batch {
		// Reserve the next position BEFORE I/O. A hung carrier, cancellation or
		// worker restart must not repeatedly strand everything after this item.
		if err := m.advance(ctx, session, "refresh", entry.TrackingNumber); err != nil {
			return err
		}
		// Recover any saved result not yet committed by an interrupted poll.
		// An unseen item instead baselines from its first successful refresh.
		if err := m.observe(ctx, session, entry, false); err != nil {
			return err
		}
		lookupCtx, cancel := context.WithTimeout(ctx, settings.LookupTimeout)
		updated, refreshErr := m.service.Refresh(lookupCtx, entry.TrackingNumber)
		cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
		if refreshErr != nil {
			failures = append(failures, fmt.Errorf("refresh %s shipment: %w", tracking.DetectCarrier(entry.TrackingNumber), refreshErr))
			continue
		}
		if err := m.observe(ctx, session, *updated, true); err != nil {
			return err
		}
	}

	batch, err = m.batch(session, "saved", entries, settings.MaxObservations)
	if err != nil {
		return err
	}
	for _, entry := range batch {
		if err := m.advance(ctx, session, "saved", entry.TrackingNumber); err != nil {
			return err
		}
		// Re-read, never compare the pre-refresh listing against a newer baseline.
		// Includes externally refreshed deliveries that aged out of carrier polling.
		latest, err := m.service.Get(ctx, entry.TrackingNumber)
		if err != nil {
			failures = append(failures, fmt.Errorf("read saved shipment: %w", err))
			continue
		}
		if !selected(*latest) {
			continue
		}
		if err := m.observe(ctx, session, *latest, true); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d shipment operations failed; successful observations committed: %w", len(failures), errors.Join(failures...))
	}

	return nil
}

func (m *monitor) batch(session *monitord.Session[State], queue string, entries []tracking.Shipment, limit int) ([]tracking.Shipment, error) {
	var previous cursor
	if _, err := session.Checkpoint(m.queueKey(queue), &previous); err != nil {
		return nil, err
	}
	// Shop lists by tracking number. A lexical cursor survives additions/removals.
	start := 0
	for start < len(entries) && entries[start].TrackingNumber <= previous.After {
		start++
	}
	batch := make([]tracking.Shipment, 0, min(limit, len(entries)))
	for offset := 0; offset < min(limit, len(entries)); offset++ {
		batch = append(batch, entries[(start+offset)%len(entries)])
	}

	return batch, nil
}

func (m *monitor) advance(ctx context.Context, session *monitord.Session[State], queue, number string) error {
	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		return tx.Checkpoint(m.queueKey(queue), cursor{After: number})
	})
}

func (m *monitor) queueKey(queue string) string {
	return "shipments:" + m.scope + ":queue:" + queue
}

func digest(parts ...string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}
