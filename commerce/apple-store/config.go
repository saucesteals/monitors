package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type config struct {
	Storefront string
	Location   string
	StoreIDs   []string
	Parts      []string
	Interval   time.Duration
	UseProxy   bool
}

var settings = config{
	Storefront: "https://www.apple.com/shop",
	Location:   "10001",
	StoreIDs:   []string{}, // Empty watches all stores returned for this location.
	Parts:      []string{"REPLACE_WITH_PART_NUMBER"},
	Interval:   time.Minute,
}

func validate() error {
	u, err := url.Parse(settings.Storefront)
	if err != nil || u.Scheme != "https" || u.Host != "www.apple.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return fmt.Errorf("storefront must be an HTTPS www.apple.com shop URL")
	}
	if !regexp.MustCompile(`^/(?:[a-z]{2}/)?shop$`).MatchString(u.Path) {
		return fmt.Errorf("storefront must end in /shop or /<country>/shop without a trailing slash")
	}
	if strings.TrimSpace(settings.Location) == "" || len(settings.Location) > 32 {
		return fmt.Errorf("set a postal code or location for the chosen storefront")
	}
	if settings.Interval < time.Minute || settings.Interval > 24*time.Hour {
		return fmt.Errorf("interval must be between one minute and 24 hours")
	}
	if len(settings.Parts) == 0 || len(settings.Parts) > 16 || len(settings.StoreIDs) > 32 {
		return fmt.Errorf("configure 1–16 parts and at most 32 store IDs")
	}
	for _, group := range []struct {
		values []string
		format string
		label  string
	}{
		{settings.Parts, `^[A-Z0-9]{5,12}/[A-Z]$`, "part number"},
		{settings.StoreIDs, `^R[0-9]{3,5}$`, "store ID"},
	} {
		seen := make(map[string]bool)
		pattern := regexp.MustCompile(group.format)
		for _, value := range group.values {
			if !pattern.MatchString(value) || seen[value] {
				return fmt.Errorf("invalid or duplicate %s %q", group.label, value)
			}
			seen[value] = true
		}
	}

	return nil
}
