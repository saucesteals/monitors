package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"time"

	"github.com/saucesteals/monitord"

	_ "time/tzdata"
)

// State is empty; availability observations belong in durable checkpoints.
type State struct{}

type snapshot struct {
	Slots    []string `json:"slots"`
	Sequence uint64   `json:"sequence"`
}

type monitor struct {
	client  *client
	zone    *time.Location
	minutes int
	scope   string
}

func main() { monitord.Run(&monitor{}) }

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "opentable",
		Description: "Restaurant openings by date, party size, and local time window",
	}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check,
		monitord.WithTimeout(2*time.Minute), monitord.WithSecrets(proxySecret))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	var err error
	m.zone, m.minutes, err = settings.validate()
	if err != nil {
		return err
	}
	// Date and party are appended to each checkpoint key; adding either does
	// not reset existing watches. Presentation and polling are not identity.
	identity := fmt.Sprintf("%d|%s|%s|%s", settings.RestaurantID, settings.Timezone, settings.StartTime, settings.EndTime)
	m.scope = fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))[:20]
	m.client, err = newClient(env.Secrets())

	return err
}

func (m *monitor) Stop(context.Context) error {
	if m.client != nil {
		m.client.pool.CloseIdleConnections()
	}

	return nil
}

func (m *monitor) check(ctx context.Context, session *monitord.Session[State]) error {
	for _, date := range settings.Dates {
		end, err := time.ParseInLocation("2006-01-02T15:04", date+"T"+settings.EndTime, m.zone)
		if err != nil {
			return err
		}
		if !end.After(time.Now()) {
			continue
		}
		for _, party := range settings.PartySizes {
			body, err := m.client.availability(ctx, date, party, m.minutes)
			if err != nil {
				return fmt.Errorf("%s, party %d: %w", date, party, err)
			}
			slots, err := m.decode(body, date)
			if err != nil {
				return err // An invalid response must not erase the previous slots.
			}
			key := fmt.Sprintf("availability:%s:%s:%d", m.scope, date, party)
			var previous snapshot
			found, err := session.Checkpoint(key, &previous)
			if err != nil {
				return err
			}
			var opened []string
			if found {
				for _, slot := range slots {
					if !slices.Contains(previous.Slots, slot) {
						opened = append(opened, slot)
					}
				}
			}
			next := snapshot{Slots: slots, Sequence: previous.Sequence}
			if len(opened) > 0 {
				next.Sequence++
			}
			if err := session.Commit(ctx, func(tx *monitord.Tx[State]) error {
				if len(opened) > 0 {
					event := openingEvent(date, party, opened)
					event.ID = fmt.Sprintf("opentable:%s:%d", key, next.Sequence)
					if err := tx.Emit(event); err != nil {
						return err
					}
				}

				return tx.Checkpoint(key, next)
			}); err != nil {
				return err
			}
		}
	}

	return nil
}
