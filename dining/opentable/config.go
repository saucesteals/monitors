package main

import (
	"fmt"
	"net/url"
	"slices"
	"time"
)

// Replace the fictional venue and dates before running.
var settings = Config{
	RestaurantID: 0,
	ProfileURL:   "https://www.opentable.com/r/example-bistro",
	Label:        "Example Bistro",
	Timezone:     "America/New_York",
	Dates:        []string{"2026-12-12"},
	PartySizes:   []int{2, 4},
	StartTime:    "18:00",
	EndTime:      "20:30",
	Interval:     time.Minute,
}

// Config selects one restaurant and independent date/party-size watches.
type Config struct {
	RestaurantID int
	ProfileURL   string
	BootstrapURL string // Optional canonical profile that supplies anonymous CSRF.
	Label        string
	Timezone     string
	Dates        []string
	PartySizes   []int
	StartTime    string
	EndTime      string
	Interval     time.Duration
}

func (c Config) validate() (*time.Location, int, error) {
	if c.RestaurantID <= 0 || c.Label == "" || c.ProfileURL == "" || c.Timezone == "" || c.Interval < time.Second {
		return nil, 0, fmt.Errorf("configure a restaurant ID, profile URL, label, timezone, and interval of at least one second")
	}
	for _, address := range []string{c.ProfileURL, c.BootstrapURL} {
		if address == "" {
			continue
		}
		u, err := url.Parse(address)
		if err != nil || u.Scheme != "https" || u.Host != "www.opentable.com" || u.User != nil || u.Path == "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, 0, fmt.Errorf("use canonical https://www.opentable.com profile URLs without query or fragment")
		}
	}
	if len(c.Dates) == 0 || len(c.Dates) > 7 || len(c.PartySizes) == 0 || len(c.PartySizes) > 20 {
		return nil, 0, fmt.Errorf("configure 1–7 dates and 1–20 party sizes")
	}
	for i, date := range c.Dates {
		if _, err := time.Parse(time.DateOnly, date); err != nil || slices.Contains(c.Dates[:i], date) {
			return nil, 0, fmt.Errorf("dates must be unique YYYY-MM-DD values")
		}
	}
	for i, party := range c.PartySizes {
		if party < 1 || party > 20 || slices.Contains(c.PartySizes[:i], party) {
			return nil, 0, fmt.Errorf("party sizes must be unique values from 1 to 20")
		}
	}
	start, err := time.Parse("15:04", c.StartTime)
	if err != nil {
		return nil, 0, fmt.Errorf("start time must be HH:MM")
	}
	end, err := time.Parse("15:04", c.EndTime)
	if err != nil || end.Before(start) {
		return nil, 0, fmt.Errorf("end time must be HH:MM and not precede start time")
	}
	zone, err := time.LoadLocation(c.Timezone)

	return zone, int(end.Sub(start) / time.Minute), err
}
