package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type selection struct {
	Color string
	Size  string
}

type config struct {
	Country    string
	Language   string
	Product    string
	PriceGroup string
	Variants   []selection
	Interval   time.Duration
	UseProxy   bool
}

var settings = config{
	Country:    "us",
	Language:   "en",
	Product:    "REPLACE_WITH_PRODUCT_ID",
	PriceGroup: "00",
	Variants:   []selection{{Color: "REPLACE_WITH_COLOR_CODE", Size: "REPLACE_WITH_SIZE_CODE"}},
	Interval:   5 * time.Minute,
}

func validate() error {
	for _, field := range []struct{ value, pattern, name string }{
		{settings.Country, `^[a-z]{2}$`, "country"},
		{settings.Language, `^[a-z]{2}$`, "language"},
		{settings.Product, `^E[0-9]{6}-[0-9]{3}$`, "product"},
		{settings.PriceGroup, `^[0-9]{2}$`, "price group"},
	} {
		if !regexp.MustCompile(field.pattern).MatchString(field.value) {
			return fmt.Errorf("invalid %s; replace configuration placeholders", field.name)
		}
	}
	if settings.Interval < time.Minute || settings.Interval > 24*time.Hour {
		return fmt.Errorf("interval must be between one minute and 24 hours")
	}
	if len(settings.Variants) == 0 || len(settings.Variants) > 32 {
		return fmt.Errorf("configure 1–32 exact color/size pairs")
	}
	seen := make(map[string]bool)
	for _, variant := range settings.Variants {
		if !regexp.MustCompile(`^[0-9]{2}$`).MatchString(variant.Color) || !regexp.MustCompile(`^[0-9]{3}$`).MatchString(variant.Size) || seen[variant.key()] {
			return fmt.Errorf("invalid or duplicate color/size pair")
		}
		seen[variant.key()] = true
	}

	return nil
}

func (s selection) key() string {
	return strings.Join([]string{s.Color, s.Size}, ":")
}
