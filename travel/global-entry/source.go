package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/saucesteals/monitord"
	"io"
	"net/http"
	"sort"
	"time"
)

func (m *monitor) check(ctx context.Context, s *monitord.Session[State]) error {
	endpoint := fmt.Sprintf("https://ttp.cbp.dhs.gov/schedulerapi/slot-availability?locationId=%d", settings.LocationID)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://ttp.cbp.dhs.gov/schedulerui/")
	res, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("CBP request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("CBP HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return fmt.Errorf("invalid CBP response size")
	}
	var body struct {
		Slots *[]Slot `json:"availableSlots"`
	}
	if err = json.Unmarshal(b, &body); err != nil || body.Slots == nil {
		return fmt.Errorf("missing CBP availableSlots array")
	}
	now := time.Now()
	var candidates []string
	for _, v := range *body.Slots {
		if !v.Active {
			continue
		}
		start, err := time.ParseInLocation("2006-01-02T15:04", v.Start, m.zone)
		if err != nil {
			return fmt.Errorf("invalid appointment start")
		}
		end, err := time.ParseInLocation("2006-01-02T15:04", v.End, m.zone)
		if err != nil || !end.After(start) || v.Duration <= 0 || v.LocationID != settings.LocationID {
			return fmt.Errorf("invalid appointment record")
		}
		if start.After(now) && (m.before.IsZero() || start.Before(m.before)) {
			candidates = append(candidates, v.Start)
		}
	}
	sort.Strings(candidates)
	earliest := ""
	if len(candidates) > 0 {
		earliest = candidates[0]
	}
	key := fmt.Sprintf("appointments:%d:%s:%s", settings.LocationID, settings.Timezone, settings.Before)
	var old Snapshot
	found, err := s.Checkpoint(key, &old)
	if err != nil {
		return err
	}
	notify := found && earliest != "" && (old.Earliest == "" || earliest < old.Earliest)
	next := Snapshot{Earliest: earliest, Sequence: old.Sequence}
	if notify {
		next.Sequence++
	}
	return s.Commit(ctx, func(tx *monitord.Tx[State]) error {
		if notify {
			if err := tx.Emit(appointmentEvent(earliest, old.Earliest, next.Sequence)); err != nil {
				return err
			}
		}
		return tx.Checkpoint(key, next)
	})
}

func appointmentEvent(start, previous string, sequence uint64) monitord.Event {
	date, _ := time.Parse("2006-01-02T15:04", start)
	title := "An interview opened up"
	description := "**" + settings.Label + "** · Global Entry"
	fields := []monitord.EventField{
		{Name: "Date", Value: date.Format("Mon, Jan 2, 2006"), Inline: true},
		{Name: "Time", Value: date.Format("3:04 PM"), Inline: true},
	}
	if previous != "" {
		old, err := time.Parse("2006-01-02T15:04", previous)
		if err == nil {
			title = "An earlier interview is available"
			fields = append(fields, monitord.EventField{Name: "Previously", Value: old.Format("Jan 2, 2006 · 3:04 PM")})
		}
	}
	description += "\n[View appointments ↗](https://ttp.cbp.dhs.gov/schedulerui/)"
	return monitord.Event{
		ID:    fmt.Sprintf("appointment:%d:%s:%s:%s:%d", settings.LocationID, settings.Timezone, settings.Before, start, sequence),
		Title: title, Description: description, URL: "https://ttp.cbp.dhs.gov/schedulerui/", Color: 0x71B89B,
		Fields: fields, Footer: settings.Timezone, Mentions: []string{},
	}
}
