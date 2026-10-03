package main

import "time"

// Replace the fictional venue and zero ID with values from its OpenTable page.
var settings = Config{
	RestaurantID: 0,
	ProfileURL:   "https://www.opentable.com/r/example-bistro",
	Label:        "Example Bistro",
	Timezone:     "America/New_York",
	Dates:        []string{"2026-12-12", "2026-12-13"},
	PartySize:    2,
	StartTime:    "18:00",
	EndTime:      "20:30",
	Interval:     5 * time.Minute,
	UseProxy:     true,
}

// Config selects one restaurant and an inclusive restaurant-local time window.
// This adapter targets opentable.com restaurants in the NA database region.
type Config struct {
	RestaurantID int
	ProfileURL   string
	Label        string
	Timezone     string
	Dates        []string
	PartySize    int
	StartTime    string
	EndTime      string
	Interval     time.Duration
	UseProxy     bool // Requires the proxies/opentable secret; never falls back to direct.
}
