package main

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

var settings = Config{
	Interval:        time.Hour,
	InitialLookback: time.Hour,
	Window:          6 * time.Hour,
	Overlap:         time.Hour,
}

var defaultWatchlist = Watchlist{
	Rules: []Rule{
		{
			Name:              "OpenSSH",
			Products:          []Product{{Vendor: "openbsd", Product: "openssh"}},
			Keywords:          []string{"OpenSSH"},
			ReferencePrefixes: []string{"https://www.openssh.com"},
		},
	},
	MinCVSS:         7,
	IncludeUnscored: true,
	NotifyUpdates:   true,
}

// Config controls polling, independent of the editable watchlist.
type Config struct {
	Interval        time.Duration
	InitialLookback time.Duration
	Window          time.Duration
	Overlap         time.Duration
}

// Watchlist is replaceable through monitord state without rebuilding.
type Watchlist struct {
	Rules           []Rule  `json:"rules"`
	MinCVSS         float64 `json:"min_cvss"`
	IncludeUnscored bool    `json:"include_unscored"`
	NotifyUpdates   bool    `json:"notify_updates"`
}

// Rule matches any product pair, keyword, or reference prefix. Exclusions win.
type Rule struct {
	Name              string    `json:"name"`
	Products          []Product `json:"products,omitempty"`
	Keywords          []string  `json:"keywords,omitempty"`
	ReferencePrefixes []string  `json:"reference_prefixes,omitempty"`
	ExcludeKeywords   []string  `json:"exclude_keywords,omitempty"`
}

// Product is an exact, case-insensitive vendor/product pair, not a version check.
type Product struct {
	Vendor  string `json:"vendor"`
	Product string `json:"product"`
}

func watchlist(state State) Watchlist {
	if state.Watchlist != nil {
		return *state.Watchlist
	}

	return defaultWatchlist
}

func validate(list Watchlist) error {
	if len(list.Rules) == 0 || len(list.Rules) > 100 {
		return fmt.Errorf("configure between 1 and 100 matching rules")
	}
	if math.IsNaN(list.MinCVSS) || math.IsInf(list.MinCVSS, 0) || list.MinCVSS < 0 || list.MinCVSS > 10 {
		return fmt.Errorf("min_cvss must be between 0 and 10")
	}
	seen := make(map[string]bool)
	for _, rule := range list.Rules {
		if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 100 || seen[rule.Name] {
			return fmt.Errorf("rule names must be nonempty, unique, and at most 100 bytes")
		}
		seen[rule.Name] = true
		if len(rule.Products)+len(rule.Keywords)+len(rule.ReferencePrefixes) == 0 {
			return fmt.Errorf("rule %q needs at least one selector", rule.Name)
		}
		for _, product := range rule.Products {
			if strings.TrimSpace(product.Vendor) == "" || strings.TrimSpace(product.Product) == "" {
				return fmt.Errorf("rule %q needs both vendor and product", rule.Name)
			}
		}
		for _, values := range [][]string{rule.Keywords, rule.ExcludeKeywords} {
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("rule %q contains a blank keyword", rule.Name)
				}
			}
		}
		for _, prefix := range rule.ReferencePrefixes {
			u, err := url.Parse(prefix)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("rule %q needs HTTPS reference prefixes without credentials, queries, or fragments", rule.Name)
			}
		}
	}

	return nil
}
