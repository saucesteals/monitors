package main

import "time"

var settings = Config{ChannelID: "UCBJycsmduvYEL83R_U4JriQ", TitleContains: "", Interval: 5 * time.Minute}

type Config struct {
	ChannelID     string
	TitleContains string // Case-insensitive; empty accepts every upload.
	Interval      time.Duration
}

// Presentation and polling changes must not reset source progress.
func sourceIdentity() any { return []any{settings.ChannelID, settings.TitleContains} }
