package main

import (
	"fmt"
	"regexp"
	"time"
)

type config struct {
	SKU                   string
	ZIPCode               string
	RadiusMiles           float64
	StoreIDs              []string
	Interval              time.Duration
	UseProxy              bool
	NotifyQuantityChanges bool
}

var settings = config{
	SKU:         "REPLACE_WITH_SKU",
	ZIPCode:     "10001",
	RadiusMiles: 25,
	Interval:    5 * time.Minute,
}

func validate() error {
	if !regexp.MustCompile(`^[0-9]{5,10}$`).MatchString(settings.SKU) {
		return fmt.Errorf("set a numeric Best Buy SKU")
	}
	if !regexp.MustCompile(`^[0-9]{5}$`).MatchString(settings.ZIPCode) {
		return fmt.Errorf("set a five-digit US ZIP code")
	}
	if !(settings.RadiusMiles > 0 && settings.RadiusMiles <= 250) {
		return fmt.Errorf("radius must be greater than zero and at most 250 miles")
	}
	if settings.Interval < time.Minute || settings.Interval > 24*time.Hour {
		return fmt.Errorf("interval must be between one minute and 24 hours")
	}
	if len(settings.StoreIDs) > 64 {
		return fmt.Errorf("configure at most 64 store IDs")
	}
	seen := make(map[string]bool)
	for _, id := range settings.StoreIDs {
		if !regexp.MustCompile(`^[0-9]{1,8}$`).MatchString(id) || seen[id] {
			return fmt.Errorf("invalid or duplicate store ID %q", id)
		}
		seen[id] = true
	}

	return nil
}
