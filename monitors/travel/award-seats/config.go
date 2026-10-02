package main

import "time"

var settings = Config{Origin: "JFK", Destination: "LHR", Dates: []string{"2026-12-08", "2026-12-09"}, Passengers: 2, MaxPoints: 85000, MaxStops: 0, Cabins: []string{"BUSINESS", "FIRST"}, IgnoreSeatCounts: []int{}, UseProxy: false, Interval: 5 * time.Minute}

type Config struct {
	Origin, Destination             string
	Dates                           []string // YYYY-MM-DD; each route/date has its own silent baseline.
	Passengers, MaxPoints, MaxStops int
	Cabins                          []string // Every segment must match; mixed-cabin fares are excluded.
	IgnoreSeatCounts                []int    // Optional source-specific phantom-inventory suppression.
	UseProxy                        bool     // Reads the named proxies/award-seats secret when enabled.
	Interval                        time.Duration
}

func sourceIdentity() any {
	return []any{settings.Origin, settings.Destination, settings.Passengers, settings.MaxPoints, settings.MaxStops, settings.Cabins, settings.IgnoreSeatCounts}
}
