package main

import (
	"strings"
	"time"
)

// etaChanged compares carrier calendar dates and meaningful time/window shifts.
// Unknown formats stay conservative: any textual change is an update.
func etaChanged(before, after string) bool {
	before, after = strings.TrimSpace(before), strings.TrimSpace(after)
	if before == after {
		return false
	}
	oldTimes, oldKind := parseETA(before)
	newTimes, newKind := parseETA(after)
	if oldKind == "" || oldKind != newKind || len(oldTimes) != len(newTimes) {
		return true
	}
	for i, old := range oldTimes {
		current := newTimes[i]
		if old.Format(time.DateOnly) != current.Format(time.DateOnly) {
			return true
		}
		if !old.Equal(current) && current.Sub(old).Abs() >= settings.ETAMinimumChange {
			return true
		}
	}

	return false
}

func parseETA(value string) ([]time.Time, string) {
	// Keep carrier-local windows separate from offset-bearing instants: the
	// timezone-free parse below is a comparison coordinate, never claimed UTC.
	if day, window, ok := strings.Cut(value, " · "); ok {
		window = strings.TrimSuffix(window, " (carrier local time)")
		parts := strings.Split(window, "–")
		for i := range parts {
			parts[i] = day + " " + strings.TrimSpace(parts[i])
		}

		return parseTimes(parts, "Mon, Jan 2, 2006 3:04 PM", "local")
	}
	if times, kind := parseTimes(strings.Split(value, " – "), "Mon, Jan 2, 2006 3:04 PM -07:00", "offset"); kind != "" {
		return times, kind
	}
	for _, layout := range []string{time.DateOnly, "Mon, Jan 2, 2006", "January 2, 2006", "Jan 2, 2006", "1/2/2006"} {
		if times, kind := parseTimes([]string{value}, layout, "date"); kind != "" {
			return times, kind
		}
	}

	return nil, ""
}

func parseTimes(parts []string, layout, kind string) ([]time.Time, string) {
	times := make([]time.Time, 0, len(parts))
	for _, part := range parts {
		parsed, err := time.Parse(layout, strings.TrimSpace(part))
		if err != nil {
			return nil, ""
		}
		times = append(times, parsed)
	}

	return times, kind
}
