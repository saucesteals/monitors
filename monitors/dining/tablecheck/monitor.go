package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/saucesteals/monitord"
	"net/http"
	"regexp"
	"time"
	_ "time/tzdata"
)

type State struct{}
type Snapshot struct {
	Available bool
	Sequence  uint64
}

type monitor struct {
	client *http.Client
	zone   *time.Location
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "tablecheck", Description: "A requested restaurant time reopens"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(time.Minute))
}

func (m *monitor) Start(context.Context, monitord.Environment) error {
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(settings.Shop) || settings.Label == "" || settings.Adults < 1 || settings.Children < 0 || settings.Infants < 0 || len(settings.Times) == 0 || len(settings.Times) > 20 || settings.Interval < time.Minute {
		return fmt.Errorf("invalid reservation configuration")
	}
	var err error
	m.zone, err = time.LoadLocation(settings.Timezone)
	if err != nil {
		return err
	}
	for _, v := range settings.Times {
		if _, err := time.ParseInLocation("2006-01-02T15:04", v, m.zone); err != nil {
			return err
		}
	}
	m.client = &http.Client{Timeout: 20 * time.Second, Transport: http.DefaultTransport.(*http.Transport).Clone()}
	return nil
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
	for _, local := range settings.Times {
		at, _ := time.ParseInLocation("2006-01-02T15:04", local, m.zone)
		if !at.After(time.Now()) {
			continue
		}
		available, err := m.fetch(ctx, at)
		if err != nil {
			return err
		}
		key := "slot:" + scope + ":" + local
		var old Snapshot
		found, err := s.Checkpoint(key, &old)
		if err != nil {
			return err
		}
		notify := found && available && !old.Available
		next := Snapshot{Available: available, Sequence: old.Sequence}
		if notify {
			next.Sequence++
		}
		if err := s.Commit(ctx, func(tx *monitord.Tx[State]) error {
			if notify {
				event := reservationEvent(at)
				event.ID = fmt.Sprintf("tablecheck:%s:%s:%d", scope, local, next.Sequence)
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
