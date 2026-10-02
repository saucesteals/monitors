package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/saucesteals/monitord"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (m *monitor) fetch(ctx context.Context, at time.Time) (bool, error) {
	q := url.Values{"reservation[start_at_epoch]": {strconv.FormatInt(at.Unix(), 10)}, "reservation[num_people_adult]": {strconv.Itoa(settings.Adults)}, "reservation[num_people_child]": {strconv.Itoa(settings.Children)}, "reservation[num_people_baby]": {strconv.Itoa(settings.Infants)}, "reservation[num_people_senior]": {"0"}}
	if settings.ServiceCategory != "" {
		q.Set("reservation[service_category]", settings.ServiceCategory)
	}
	if settings.MenuID != "" {
		q.Set("reservation[orders_attributes][0][menu_item_id]", settings.MenuID)
		q.Set("reservation[orders_attributes][0][is_group_order]", strconv.FormatBool(settings.GroupOrder))
		if !settings.GroupOrder {
			q.Set("reservation[orders_attributes][0][quantity]", strconv.Itoa(settings.Adults))
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.tablecheck.com/en/shops/"+settings.Shop+"/available?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "monitord-tablecheck")
	res, err := m.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("TableCheck request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return false, fmt.Errorf("TableCheck HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return false, fmt.Errorf("invalid response size")
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return false, err
	}
	switch result.Status {
	case "success":
		return true, nil
	case "closed", "same_day", "other_day", "service_category", "reservation_request":
		return false, nil
	default:
		return false, fmt.Errorf("unconfirmed availability status %q", result.Status)
	}
}

func reservationEvent(at time.Time) monitord.Event {
	party := fmt.Sprintf("%d adults", settings.Adults)
	if settings.Children > 0 {
		party += fmt.Sprintf(" · %d children", settings.Children)
	}
	if settings.Infants > 0 {
		party += fmt.Sprintf(" · %d infants", settings.Infants)
	}
	return monitord.Event{ID: "example:tablecheck", Title: settings.Label, Description: "**A table opened up**\n[Reserve this time ↗](https://www.tablecheck.com/en/shops/" + settings.Shop + "/reserve)", URL: "https://www.tablecheck.com/en/shops/" + settings.Shop + "/reserve", Image: settings.Image, Color: 0xC4A4DA, Fields: []monitord.EventField{{Name: "Date", Value: at.Format("Mon, Jan 2, 2006"), Inline: true}, {Name: "Time", Value: at.Format("3:04 PM MST"), Inline: true}, {Name: "Party", Value: party, Inline: false}}, Mentions: []string{}}
}
