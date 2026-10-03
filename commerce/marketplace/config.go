package main

import (
	"fmt"
	"regexp"
	"time"
)

var settings = Config{City: "austin", Radius: "25mi", Queries: []string{"Herman Miller Aeron", "Herman Miller Mirra"}, Include: `(?i)\b(aeron|mirra)\b`, Exclude: `(?i)\b(parts|repair|wanted|replica)\b`, MinCents: 10000, MaxCents: 45000, Currency: "USD", Days: 1, Pages: 2, Interval: 2 * time.Minute}

type Config struct {
	City, Radius       string
	Queries            []string
	Include, Exclude   string // RE2 patterns; empty disables that filter.
	MinCents, MaxCents int64
	Currency           string
	Days, Pages        int
	Interval           time.Duration
}

func sourceIdentity() any {
	return []any{settings.City, settings.Radius, settings.Queries, settings.Include, settings.Exclude, settings.MinCents, settings.MaxCents, settings.Currency, settings.Days, settings.Pages}
}

func validate() error {
	if settings.City == "" || len(settings.Queries) == 0 || settings.Pages < 1 || settings.Pages > 10 || settings.Days < 1 || settings.Days > 365 || settings.Interval < time.Minute || settings.MinCents < 0 || settings.MaxCents < settings.MinCents {
		return fmt.Errorf("invalid search configuration")
	}
	for _, p := range []string{settings.Include, settings.Exclude} {
		if _, err := regexp.Compile(p); err != nil {
			return err
		}
	}
	return nil
}
