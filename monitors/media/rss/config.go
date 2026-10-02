package main

import "time"

var settings = Config{URL: "https://blog.cloudflare.com/rss/", TitleContains: "", Category: "", Interval: 10 * time.Minute}

type Config struct {
	URL           string
	TitleContains string
	Category      string // Exact category/tag; empty watches every category.
	Interval      time.Duration
}

// Presentation and polling changes must not reset source progress.
func sourceIdentity() any { return []any{settings.URL, settings.TitleContains, settings.Category} }
