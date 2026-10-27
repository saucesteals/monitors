package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/saucesteals/monitord"
)

// State is empty; source observations and alert revisions live in checkpoints.
type State struct{}

type observation struct {
	Name      string `json:"name"`
	Image     string `json:"image,omitempty"`
	Available bool   `json:"available"`
}

type snapshot struct {
	Listings map[string]observation `json:"listings"`
	Revision uint64                 `json:"revision"`
}

type monitor struct {
	source *source
	scope  string
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "airbnb",
		Description: "Matching Airbnb stays with full-date quote verification",
	}
}

func (m *monitor) Plan() monitord.Plan[State] {
	options := []monitord.CommonOption{monitord.WithTimeout(10 * time.Minute)}
	if settings.UseProxy {
		options = append(options, monitord.WithSecrets(proxySecret))
	}

	return monitord.Every(settings.Interval, m.check, options...)
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	if err := settings.validate(); err != nil {
		return err
	}
	// Cadence, transport and safety bounds do not change the watched condition.
	identity := settings
	identity.Interval = 0
	identity.MaxPages = 0
	identity.MaxListings = 0
	identity.UseProxy = false
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	m.scope = fmt.Sprintf("%x", sha256.Sum256(raw))[:24]
	m.source, err = newSource(settings, env.Secrets())

	return err
}

func (m *monitor) Stop(context.Context) error {
	if m.source != nil {
		m.source.client.CloseIdleConnections()
	}

	return nil
}

func (m *monitor) check(ctx context.Context, session *monitord.Session[State]) error {
	// Dates are civil dates. Stop conservatively once check-in starts in UTC;
	// this example does not support same-day watches across local timezones.
	if settings.CheckIn <= time.Now().UTC().Format(time.DateOnly) {
		return fmt.Errorf("stay has reached check-in; update configuration or retire this deployment")
	}
	key := "availability-v1:" + m.scope
	var previous snapshot
	baseline, err := session.Checkpoint(key, &previous)
	if err != nil {
		return err
	}
	current, err := m.source.observe(ctx, previous.Listings)
	if err != nil {
		// All-or-nothing: never establish a partial baseline or turn a failed
		// quote, incomplete search, or disappeared card into unavailability.
		m.source.invalidate()

		return err
	}

	ids := make([]string, 0, len(current))
	for id := range current {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		next := snapshot{
			Listings: make(map[string]observation, len(current)),
			Revision: previous.Revision,
		}
		for _, id := range ids {
			result := current[id]
			next.Listings[id] = result.observation
			if !baseline || !result.Available || previous.Listings[id].Available {
				continue
			}
			next.Revision++
			event := availabilityEvent(id, result)
			event.ID = fmt.Sprintf("airbnb:%s:%d", m.scope, next.Revision)
			if err := tx.Emit(event); err != nil {
				return err
			}
		}
		slog.InfoContext(ctx, "airbnb observation complete", "listings", len(current), "baseline", !baseline, "events", next.Revision-previous.Revision)

		return tx.Checkpoint(key, next)
	})
}

func availabilityEvent(id string, result result) monitord.Event {
	name := result.Name
	if name == "" {
		name = "Listing " + id
	}

	return monitord.Event{
		Title:       "Matching stay available · " + name,
		URL:         settings.listingURL(id),
		Description: fmt.Sprintf("%s → %s · %d adults", settings.CheckIn, settings.CheckOut, settings.Adults),
		Image:       result.Image,
		Color:       0xFF385C,
		Fields: []monitord.EventField{
			{Name: "Quote details (" + settings.Currency + ")", Value: result.Price},
		},
		Footer: "Airbnb web quote · availability can change · no reservation made",
	}
}
