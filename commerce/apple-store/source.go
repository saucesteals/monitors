package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
)

type product struct {
	PickupDisplay string `json:"pickupDisplay"`
	MessageTypes  struct {
		Regular struct {
			Enabled *bool  `json:"storeSelectionEnabled"`
			Title   string `json:"storePickupProductTitle"`
			Quote   string `json:"storePickupQuote"`
		} `json:"regular"`
	} `json:"messageTypes"`
}

type store struct {
	Number string             `json:"storeNumber"`
	Name   string             `json:"storeName"`
	Parts  map[string]product `json:"partsAvailability"`
}

type response struct {
	Head struct {
		Status string `json:"status"`
	} `json:"head"`
	Body struct {
		ErrorMessage string  `json:"errorMessage"`
		Stores       []store `json:"stores"`
	} `json:"body"`
}

func (m *monitor) fetch(ctx context.Context) ([]store, time.Duration, error) {
	query := url.Values{
		"pl":       {"true"},
		"location": {settings.Location},
	}
	for i, part := range settings.Parts {
		query.Set(fmt.Sprintf("parts.%d", i), part)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.Storefront+"/retail/pickup-message?"+query.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		// Avoid leaking proxy credentials through transport errors.
		return nil, 0, fmt.Errorf("apple pickup request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, retryAfter(res.Header.Get("Retry-After")), fmt.Errorf("apple HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, 0, fmt.Errorf("apple response unreadable or larger than 4 MiB")
	}
	var data response
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, 0, fmt.Errorf("decode apple response: %w", err)
	}
	if data.Head.Status != "200" || data.Body.ErrorMessage != "" || len(data.Body.Stores) == 0 {
		return nil, 0, fmt.Errorf("apple returned an error or no stores; check location and part numbers")
	}

	selected := make([]store, 0, len(data.Body.Stores))
	seen := make(map[string]bool)
	for _, store := range data.Body.Stores {
		if store.Number == "" || seen[store.Number] {
			return nil, 0, fmt.Errorf("missing or duplicate store ID")
		}
		seen[store.Number] = true
		if len(settings.StoreIDs) > 0 && !slices.Contains(settings.StoreIDs, store.Number) {
			continue
		}
		if strings.TrimSpace(store.Name) == "" {
			return nil, 0, fmt.Errorf("missing store name for %s", store.Number)
		}
		for _, part := range settings.Parts {
			product, ok := store.Parts[part]
			regular := product.MessageTypes.Regular
			if !ok || regular.Enabled == nil || strings.TrimSpace(regular.Title) == "" {
				return nil, 0, fmt.Errorf("missing product data for %s/%s", store.Number, part)
			}
			if (product.PickupDisplay != "available" && product.PickupDisplay != "unavailable") || *regular.Enabled != (product.PickupDisplay == "available") {
				return nil, 0, fmt.Errorf("ambiguous availability for %s/%s", store.Number, part)
			}
		}
		selected = append(selected, store)
	}
	for _, id := range settings.StoreIDs {
		if !seen[id] {
			return nil, 0, fmt.Errorf("required store %s absent from location results", id)
		}
	}
	slices.SortFunc(selected, func(a, b store) int { return strings.Compare(a.Number, b.Number) })

	return selected, 0, nil
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		// Ignore values that cannot be represented as a duration.
		if seconds <= int64((1<<63-1)/time.Second) {
			return time.Duration(seconds) * time.Second
		}
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(date))
	}

	return 0
}

func pickupEvent(store store, part string) monitord.Event {
	regular := store.Parts[part].MessageTypes.Regular
	link := settings.Storefront + "/product/" + url.PathEscape(part)
	quote := strings.TrimSpace(regular.Quote)
	if quote == "" {
		quote = "Available for pickup"
	}

	return monitord.Event{
		Title:       clip(regular.Title, 200),
		Description: "Pickup available at Apple " + clip(store.Name, 150),
		URL:         link,
		Color:       0x34C759,
		Fields: []monitord.EventField{
			{Name: "Store", Value: clip(store.Name, 150) + " · " + store.Number, Inline: true},
			{Name: "Pickup", Value: clip(quote, 500), Inline: true},
			{Name: "Part number", Value: part, Inline: true},
		},
		Footer: "Apple Store pickup • Availability is not a reservation",
	}
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}

	return value
}
