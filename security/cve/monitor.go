package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/saucesteals/monitord"
)

// State holds only operator configuration; source progress lives in checkpoints.
type State struct {
	Watchlist *Watchlist `json:"watchlist,omitempty"`
}

// Cursor marks a completed modification window and the fixed silent baseline.
type Cursor struct {
	Through       time.Time `json:"through"`
	BaselineUntil time.Time `json:"baseline_until"`
}

// Observation suppresses repeated content while allowing A -> B -> A changes.
type Observation struct {
	Fingerprint string    `json:"fingerprint"`
	Modified    time.Time `json:"modified"`
	Sequence    uint64    `json:"sequence"`
	Eligible    bool      `json:"eligible"`
}

type monitor struct {
	client      *http.Client
	apiKey      string
	nextRequest time.Time
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "cve",
		Description: "New and materially changed CVEs matching a configurable watchlist",
	}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check,
		monitord.WithTimeout(10*time.Minute), monitord.WithSecrets(nvdAPIKey))
}

func (m *monitor) Start(_ context.Context, env monitord.Environment) error {
	if settings.Interval < time.Minute || settings.InitialLookback <= 0 || settings.Window <= 0 || settings.Overlap < 0 || settings.Window+settings.Overlap > 120*24*time.Hour {
		return fmt.Errorf("invalid polling durations; window plus overlap must not exceed 120 days")
	}
	if err := validate(defaultWatchlist); err != nil {
		return err
	}
	m.apiKey, _ = env.Secrets().Get(nvdAPIKey)
	m.client = &http.Client{
		Timeout:   90 * time.Second,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("unexpected NVD redirect")
		},
	}

	return nil
}

func (m *monitor) Stop(context.Context) error {
	if m.client != nil {
		m.client.CloseIdleConnections()
	}

	return nil
}

func scopeFor(list Watchlist) string {
	// Rule labels/order and delivery/polling settings do not reset progress.
	identities := make([]string, 0, len(list.Rules))
	for _, rule := range list.Rules {
		rule.Name = ""
		raw, _ := json.Marshal(rule)
		identities = append(identities, string(raw))
	}
	slices.Sort(identities)
	raw, _ := json.Marshal(struct {
		Rules           []string
		MinCVSS         float64
		IncludeUnscored bool
		NotifyUpdates   bool
	}{identities, list.MinCVSS, list.IncludeUnscored, list.NotifyUpdates})

	return fmt.Sprintf("%x", sha256.Sum256(raw))[:24]
}

func (m *monitor) check(ctx context.Context, session *monitord.Session[State]) error {
	list := watchlist(session.State())
	if err := validate(list); err != nil {
		return err
	}
	scope := scopeFor(list)
	key := "nvd:" + scope
	var cursor Cursor
	found, err := session.Checkpoint(key, &cursor)
	if err != nil {
		return err
	}
	// Leave a small indexing lag and replay an overlap on every completed poll.
	now := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Millisecond)
	if !found {
		cursor = Cursor{
			Through:       now.Add(-settings.InitialLookback),
			BaselineUntil: now,
		}
		// Persist the initial cutoff before I/O so retries cannot swallow new CVEs.
		if err := session.Commit(ctx, func(tx *monitord.Tx[State]) error {
			if scopeFor(watchlist(*tx.State)) != scope {
				return fmt.Errorf("watchlist changed during poll")
			}

			return tx.Checkpoint(key, cursor)
		}); err != nil {
			return err
		}
	}
	if !cursor.Through.Before(now) {
		return nil
	}
	end := cursor.Through.Add(settings.Window)
	if end.After(now) {
		end = now
	}
	if cursor.Through.Before(cursor.BaselineUntil) && end.After(cursor.BaselineUntil) {
		end = cursor.BaselineUntil
	}
	baseline := !end.After(cursor.BaselineUntil)
	count, err := m.scan(ctx, cursor.Through.Add(-settings.Overlap), end, func(cve CVE) error {
		return observe(ctx, session, scope, list, cve, baseline)
	})
	if err != nil {
		return err
	}
	cursor.Through = end
	if err := session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		if scopeFor(watchlist(*tx.State)) != scope {
			return fmt.Errorf("watchlist changed during poll")
		}

		return tx.Checkpoint(key, cursor)
	}); err != nil {
		return err
	}
	slog.InfoContext(ctx, "completed cve window", "records", count, "through", end, "baseline", baseline)

	return nil
}

func observe(ctx context.Context, session *monitord.Session[State], scope string, list Watchlist, cve CVE, baseline bool) error {
	key := "cve:" + scope + ":" + cve.ID
	var previous Observation
	found, err := session.Checkpoint(key, &previous)
	if err != nil {
		return err
	}
	modified, err := parseTimestamp(cve.LastModified)
	if err != nil {
		return err
	}
	if found && modified.Before(previous.Modified) {
		return nil
	}
	detail := summarize(cve)
	labels := matchingRules(list.Rules, detail)
	if len(labels) == 0 && !found {
		return nil
	}
	eligible := len(labels) > 0 && !detail.Rejected && ((detail.Score == nil && list.IncludeUnscored) || (detail.Score != nil && *detail.Score >= list.MinCVSS))
	raw, _ := json.Marshal(detail)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(raw))
	if found && previous.Fingerprint == fingerprint && previous.Eligible == eligible {
		if !modified.After(previous.Modified) {
			return nil
		}
		previous.Modified = modified

		return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
			if scopeFor(watchlist(*tx.State)) != scope {
				return fmt.Errorf("watchlist changed during poll")
			}

			return tx.Checkpoint(key, previous)
		})
	}
	next := Observation{
		Fingerprint: fingerprint,
		Modified:    modified,
		Sequence:    previous.Sequence + 1,
		Eligible:    eligible,
	}
	// Withdrawal is useful even though rejected records no longer match filters.
	withdrawn := found && previous.Eligible && detail.Rejected
	emit := !baseline && ((eligible && (!found || !previous.Eligible || list.NotifyUpdates)) || (found && previous.Eligible && list.NotifyUpdates))
	event := cveEvent(cve, detail, labels, found, withdrawn)
	event.ID = fmt.Sprintf("cve:%s:%s:%d", scope, cve.ID, next.Sequence)

	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		if scopeFor(watchlist(*tx.State)) != scope {
			return fmt.Errorf("watchlist changed during poll")
		}
		if emit {
			if err := tx.Emit(event); err != nil {
				return err
			}
		}

		return tx.Checkpoint(key, next)
	})
}
