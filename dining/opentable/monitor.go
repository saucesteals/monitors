package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/saucesteals/monitord"

	_ "time/tzdata" // Keep restaurant timezones available on minimal hosts.
)

// State is empty; availability observations belong in durable checkpoints.
type State struct{}

type snapshot struct {
	Slots    []string `json:"slots"`
	Sequence uint64   `json:"sequence"`
}

type monitor struct {
	sessions []*browserSession
	active   int
	zone     *time.Location
	scope    string
	minutes  int
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "opentable", Description: "Restaurant openings within your dinner window"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	opts := []monitord.CommonOption{monitord.WithTimeout(2 * time.Minute)}
	if settings.UseProxy {
		opts = append(opts, monitord.WithSecrets(proxySecret))
	}

	return monitord.Every(settings.Interval, m.check, opts...)
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	profile, err := url.Parse(settings.ProfileURL)
	if err != nil || profile.Scheme != "https" || profile.Host != "www.opentable.com" || profile.User != nil || profile.RawQuery != "" || profile.Fragment != "" || !regexp.MustCompile(`^/(r/)?[a-z0-9-]+$`).MatchString(profile.Path) {
		return fmt.Errorf("profile URL must be a canonical https://www.opentable.com restaurant URL without query parameters")
	}
	if settings.RestaurantID <= 0 || settings.Label == "" || settings.PartySize < 1 || settings.PartySize > 20 || len(settings.Dates) == 0 || len(settings.Dates) > 7 || settings.Interval < time.Minute {
		return fmt.Errorf("configure a restaurant ID, profile URL, label, 1–7 dates, party of 1–20, and interval of at least one minute")
	}
	m.zone, err = time.LoadLocation(settings.Timezone)
	if err != nil {
		return fmt.Errorf("restaurant timezone: %w", err)
	}
	start, err := time.Parse("15:04", settings.StartTime)
	if err != nil {
		return fmt.Errorf("start time: %w", err)
	}
	end, err := time.Parse("15:04", settings.EndTime)
	if err != nil || end.Before(start) {
		return fmt.Errorf("end time must be HH:MM and not precede start time")
	}
	m.minutes = int(end.Sub(start) / time.Minute)
	seen := make(map[string]bool)
	for _, date := range settings.Dates {
		if _, err := time.Parse(time.DateOnly, date); err != nil || seen[date] {
			return fmt.Errorf("dates must be unique YYYY-MM-DD values")
		}
		seen[date] = true
	}
	// Presentation and polling frequency do not change source identity.
	identity := struct {
		ID    int
		Zone  string
		Party int
		Start string
		End   string
	}{settings.RestaurantID, settings.Timezone, settings.PartySize, settings.StartTime, settings.EndTime}
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	m.scope = fmt.Sprintf("%x", sha256.Sum256(raw))[:20]
	return m.startSessions(env.Secrets())
}

func (m *monitor) Stop(context.Context) error {
	for _, session := range m.sessions {
		session.client.CloseIdleConnections()
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
		slots, err := m.fetch(ctx, date)
		if err != nil {
			return fmt.Errorf("availability for %s: %w", date, err)
		}
		key := "availability:" + m.scope + ":" + date
		var previous snapshot
		found, err := session.Checkpoint(key, &previous)
		if err != nil {
			return err
		}
		next := snapshot{Slots: slots, Sequence: previous.Sequence}
		var opened []string
		if found {
			for _, slot := range slots {
				if !slices.Contains(previous.Slots, slot) {
					opened = append(opened, slot)
				}
			}
		}
		if len(opened) > 0 {
			next.Sequence++
		}
		// Poll callbacks are serialized. Save the observation and its single
		// per-date event atomically; a failed fetch never rearms an opening.
		if err := session.Commit(ctx, func(tx *monitord.Tx[State]) error {
			if len(opened) > 0 {
				event := openingEvent(date, opened)
				event.ID = fmt.Sprintf("opentable:%s:%s:%d", m.scope, date, next.Sequence)
				if err := tx.Emit(event); err != nil {
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
