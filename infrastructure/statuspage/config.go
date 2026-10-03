package main

import "time"

var settings = Config{URL: "https://www.githubstatus.com", Components: []string{}, MinimumImpact: "none", Interval: time.Minute}

type Config struct {
	URL           string
	Components    []string // Exact component names; empty watches every component.
	MinimumImpact string   // none, minor, major, critical
	Interval      time.Duration
}

// Presentation and polling changes must not reset source progress.
func sourceIdentity() any { return []any{settings.URL, settings.Components, settings.MinimumImpact} }
