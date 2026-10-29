package main

import (
	"fmt"
	"time"
)

var settings = Config{
	City: "burlingtonct", Radius: "20mi",
	Queries:  []string{"firewood", "free firewood", "seasoned wood", "free logs", "wood logs", "log rounds"},
	MinCents: 0, MaxCents: 100000000, Currency: "USD",
	Days: 1, Pages: 10, Interval: 5 * time.Minute,
}

type Config struct {
	City, Radius       string
	Queries            []string
	MinCents, MaxCents int64
	Currency           string
	Days, Pages        int
	Interval           time.Duration
}

func sourceIdentity() any {
	return []any{settings.City, settings.Radius, settings.Queries, settings.MinCents, settings.MaxCents, settings.Currency, settings.Days, settings.Pages}
}

func validate() error {
	if settings.City == "" || len(settings.Queries) == 0 || settings.Pages < 1 || settings.Pages > 10 || settings.Days < 1 || settings.Days > 365 || settings.Interval < time.Minute || settings.MinCents < 0 || settings.MaxCents < settings.MinCents {
		return fmt.Errorf("invalid search configuration")
	}
	return nil
}
