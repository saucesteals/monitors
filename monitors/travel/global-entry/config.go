package main

import "time"

var settings = Config{
	LocationID: 5140,
	Label:      "JFK International",
	Timezone:   "America/New_York",
	Before:     "2027-01-01", // Exclusive local date. Empty accepts any future opening.
	Interval:   time.Minute,
}

type Config struct {
	LocationID int
	Label      string
	Timezone   string
	Before     string
	Interval   time.Duration
}
