package main

import (
	"fmt"
	"strings"
	"time"
)

var settings = Config{
	Latitude: 41.7692, Longitude: -72.965, Radius: "20mi",
	Queries:  []string{"firewood"},
	MinCents: 0, MaxCents: 100000000,
	Days: 1, Interval: 30 * time.Second,
}

type Config struct {
	Latitude, Longitude float64
	Radius              string
	Queries             []string
	MinCents, MaxCents  int64
	Days                int
	Interval            time.Duration
}

func sourceIdentity() any {
	return []any{"shop-coordinates-first-page-v2", settings.Latitude, settings.Longitude, settings.Radius, settings.Queries, settings.MinCents, settings.MaxCents, settings.Days}
}

func validate() error {
	if !(settings.Latitude >= -90 && settings.Latitude <= 90 && settings.Longitude >= -180 && settings.Longitude <= 180) || strings.TrimSpace(settings.Radius) == "" || len(settings.Queries) == 0 || settings.Days < 1 || settings.Days > 365 || settings.Interval < 30*time.Second || settings.MinCents < 0 || settings.MaxCents < settings.MinCents {
		return fmt.Errorf("invalid search configuration")
	}
	seen := make(map[string]bool)
	for _, query := range settings.Queries {
		key := strings.ToLower(strings.TrimSpace(query))
		if key == "" || seen[key] {
			return fmt.Errorf("queries must be nonempty and distinct")
		}
		seen[key] = true
	}
	return nil
}
