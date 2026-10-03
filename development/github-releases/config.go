package main

import "time"

var settings = Config{Repository: "cli/cli", IncludePrereleases: false, TagPrefix: "", Interval: 5 * time.Minute}

type Config struct {
	Repository         string
	IncludePrereleases bool
	TagPrefix          string
	Interval           time.Duration
}

// Presentation and polling changes must not reset source progress.
func sourceIdentity() any {
	return []any{settings.Repository, settings.IncludePrereleases, settings.TagPrefix}
}
