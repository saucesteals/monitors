package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/shop/tracking"
)

type observation struct {
	Scans    string `json:"scans"`
	ETA      string `json:"eta"`
	Sequence uint64 `json:"sequence"`
}

func (m *monitor) observe(ctx context.Context, session *monitord.Session[State], shipment tracking.Shipment, baseline bool) error {
	snapshot := shipment.Tracking
	if snapshot == nil || snapshot.TrackingNumber != shipment.TrackingNumber || snapshot.Latest() == nil {
		return nil
	}
	// AddedAt separates a removed/re-added number from its earlier shipment.
	key := "shipments:" + m.scope + ":item:" + digest(shipment.TrackingNumber, shipment.AddedAt.Format(time.RFC3339Nano))
	var previous observation
	known, err := session.Checkpoint(key, &previous)
	if err != nil {
		return err
	}
	if !known && !baseline {
		return nil
	}
	encoded, err := json.Marshal(snapshot.Events)
	if err != nil {
		return fmt.Errorf("encode scan history: %w", err)
	}
	next := observation{
		Scans:    digest(string(encoded)),
		ETA:      strings.TrimSpace(snapshot.ExpectedDelivery),
		Sequence: previous.Sequence,
	}
	changed := next.Scans != previous.Scans || etaChanged(previous.ETA, next.ETA)
	if known && !changed {
		// Do not move the ETA anchor for sub-threshold shifts: drift accumulates.
		return nil
	}
	var event monitord.Event
	if known {
		next.Sequence++
		event = shipmentEvent(shipment, previous, next)
		event.ID = fmt.Sprintf("%s:%d", key, next.Sequence)
	}
	// Poll callbacks are serial. No network or other Commit occurs between this
	// checkpoint read and write; the occurrence and its baseline commit together.
	return session.Commit(ctx, func(tx *monitord.Tx[State]) error {
		if known {
			if err := tx.Emit(event); err != nil {
				return err
			}
		}

		return tx.Checkpoint(key, next)
	})
}

func shipmentEvent(shipment tracking.Shipment, previous, next observation) monitord.Event {
	latest := shipment.Tracking.Latest()
	title := strings.TrimSpace(shipment.Label)
	if title == "" {
		title = "Package " + shipment.TrackingNumber
	}
	reason := "Tracking updated"
	if previous.Scans == next.Scans {
		reason = "Delivery estimate changed"
	}
	fields := []monitord.EventField{{
		Name:  "Latest scan",
		Value: clip(latest.Description, 700),
	}}
	if stamp := strings.TrimSpace(latest.Date + " " + latest.Time); stamp != "" {
		fields = append(fields, monitord.EventField{
			Name:  "Scan time",
			Value: clip(stamp, 700),
		})
	}
	if latest.Location != "" {
		fields = append(fields, monitord.EventField{
			Name:  "Location",
			Value: clip(latest.Location, 700),
		})
	}
	estimate := next.ETA
	if estimate == "" {
		estimate = "Not provided by the carrier"
	}
	fields = append(fields, monitord.EventField{
		Name:  "Expected delivery",
		Value: clip(estimate, 700),
	})
	if previous.ETA != "" && etaChanged(previous.ETA, next.ETA) {
		fields = append(fields, monitord.EventField{
			Name:  "Previous estimate",
			Value: clip(previous.ETA, 700),
		})
	}
	if shipment.Merchant != "" {
		fields = append(fields, monitord.EventField{
			Name:  "Merchant",
			Value: clip(shipment.Merchant, 700),
		})
	}

	return monitord.Event{
		Title:       clip(title, 240),
		Description: reason,
		URL:         shipment.Tracking.URL,
		Fields:      fields,
		Footer:      "Carrier estimate, not a guarantee · Source freshness may be unknown",
		Mentions:    []string{},
	}
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}

	return value
}
