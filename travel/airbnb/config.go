package main

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type area struct {
	North float64
	South float64
	East  float64
	West  float64
}

type config struct {
	Destination  string // Airbnb search-path slug; the rectangle defines the area.
	CheckIn      string
	CheckOut     string
	Adults       int
	Currency     string
	Area         area
	MinBedrooms  float64
	MinBeds      float64
	MinBathrooms float64
	Interval     time.Duration
	MaxPages     int
	MaxListings  int
	UseProxy     bool
}

// Replace the destination, dates and rectangle before running.
var settings = config{
	Destination: "REPLACE_WITH_DESTINATION",
	CheckIn:     "YYYY-MM-DD",
	CheckOut:    "YYYY-MM-DD",
	Adults:      2,
	Currency:    "USD",
	Area:        area{},
	Interval:    time.Hour,
	MaxPages:    10,
	MaxListings: 64,
}

func (c config) validate() error {
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9,-]{0,159}$`).MatchString(c.Destination) || strings.HasPrefix(c.Destination, "REPLACE") {
		return fmt.Errorf("set a public Airbnb destination slug, for example City--Country")
	}
	start, err := time.Parse(time.DateOnly, c.CheckIn)
	if err != nil {
		return fmt.Errorf("check-in must be YYYY-MM-DD")
	}
	end, err := time.Parse(time.DateOnly, c.CheckOut)
	if err != nil || !end.After(start) || end.Sub(start) > 90*24*time.Hour {
		return fmt.Errorf("checkout must be 1–90 nights after check-in")
	}
	if c.Adults < 1 || c.Adults > 16 || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(c.Currency) {
		return fmt.Errorf("configure 1–16 adults and a three-letter currency")
	}
	a := c.Area
	for _, value := range []float64{a.North, a.South, a.East, a.West, c.MinBedrooms, c.MinBeds, c.MinBathrooms} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("coordinates and room minima must be finite")
		}
	}
	if a.North > 90 || a.South < -90 || a.East > 180 || a.West < -180 || a.North <= a.South || a.East <= a.West {
		return fmt.Errorf("configure a valid rectangle: north > south, east > west, without crossing the antimeridian")
	}
	for _, minimum := range []float64{c.MinBedrooms, c.MinBeds, c.MinBathrooms} {
		if minimum < 0 || minimum > 50 || math.Mod(minimum, 0.5) != 0 {
			return fmt.Errorf("room minima must be 0–50 in half-unit increments; zero disables a filter")
		}
	}
	if c.Interval < 15*time.Minute || c.Interval > 24*time.Hour || c.MaxPages < 1 || c.MaxPages > 20 || c.MaxListings < 1 || c.MaxListings > 128 {
		return fmt.Errorf("use a 15-minute–24-hour interval, 1–20 pages, and 1–128 retained listings")
	}

	return nil
}

func (c config) searchURL() string {
	q := url.Values{
		"checkin": {c.CheckIn}, "checkout": {c.CheckOut},
		"adults": {strconv.Itoa(c.Adults)}, "currency": {c.Currency},
		"ne_lat": {number(c.Area.North)}, "ne_lng": {number(c.Area.East)},
		"sw_lat": {number(c.Area.South)}, "sw_lng": {number(c.Area.West)},
		"search_by_map": {"true"}, "search_type": {"user_map_move"}, "locale": {"en"},
	}
	for _, filter := range []struct {
		name  string
		value float64
	}{
		{"min_bedrooms", c.MinBedrooms},
		{"min_beds", c.MinBeds},
		{"min_bathrooms", c.MinBathrooms},
	} {
		if filter.value > 0 {
			q.Set(filter.name, number(filter.value))
		}
	}

	return "https://www.airbnb.com/s/" + url.PathEscape(c.Destination) + "/homes?" + q.Encode()
}

func (c config) listingURL(id string) string {
	q := url.Values{
		"check_in": {c.CheckIn}, "check_out": {c.CheckOut},
		"adults": {strconv.Itoa(c.Adults)}, "currency": {c.Currency}, "locale": {"en"},
	}

	return "https://www.airbnb.com/rooms/" + id + "?" + q.Encode()
}

func number(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
