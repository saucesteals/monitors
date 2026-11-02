package main

import (
	"fmt"
	"time"
)

var settings = Config{
	City: "burlingtonct", Radius: "20mi",
	Queries:  []string{"firewood"},
	MinCents: 0, MaxCents: 100000000, Currency: "USD",
	Days: 1, Interval: 30 * time.Second,
}

type Config struct {
	City, Radius       string
	Queries            []string
	MinCents, MaxCents int64
	Currency           string
	Days               int
	Interval           time.Duration
}

func sourceIdentity() any {
	return []any{"first-page-native-location", settings.City, settings.Radius, settings.Queries, settings.MinCents, settings.MaxCents, settings.Currency, settings.Days}
}

func validate() error {
	if settings.City == "" || len(settings.Queries) == 0 || settings.Days < 1 || settings.Days > 365 || settings.Interval < 30*time.Second || settings.MinCents < 0 || settings.MaxCents < settings.MinCents {
		return fmt.Errorf("invalid search configuration")
	}
	return nil
}
