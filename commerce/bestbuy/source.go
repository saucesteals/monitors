package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
)

type availabilityRequest struct {
	ZIPCode               string        `json:"zipCode"`
	ShowOnShelf           bool          `json:"showOnShelf"`
	LookupInStoreQuantity bool          `json:"lookupInStoreQuantity"`
	XboxAllAccess         bool          `json:"xboxAllAccess"`
	Consolidated          bool          `json:"consolidated"`
	ShowOnlyOnShelf       bool          `json:"showOnlyOnShelf"`
	ShowInStore           bool          `json:"showInStore"`
	PickupTypes           []string      `json:"pickupTypes"`
	OnlyBestBuyLocations  bool          `json:"onlyBestBuyLocations"`
	Items                 []requestItem `json:"items"`
}

type requestItem struct {
	SKU                 string   `json:"sku"`
	Quantity            int      `json:"quantity"`
	ItemSeqNumber       string   `json:"itemSeqNumber"`
	SelectedServices    []string `json:"selectedServices"`
	RequiredAccessories []string `json:"requiredAccessories"`
	IsTradeIn           bool     `json:"isTradeIn"`
	IsLeased            bool     `json:"isLeased"`
}

type location struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Distance *float64 `json:"distance"`
}

type itemLocation struct {
	ID           string `json:"locationId"`
	Availability struct {
		Quantity *int `json:"availablePickupQuantity"`
	} `json:"availability"`
}

type availabilityResponse struct {
	Pickup *struct {
		Locations []location `json:"locations"`
		Items     []struct {
			SKU       string         `json:"sku"`
			Locations []itemLocation `json:"locations"`
		} `json:"items"`
	} `json:"ispu"`
}

type stock struct {
	ID       string
	Name     string
	Distance float64
	Quantity int
}

func productURL() string {
	return "https://www.bestbuy.com/site/" + settings.SKU + ".p?skuId=" + settings.SKU
}

func (m *monitor) fetch(ctx context.Context) ([]stock, time.Duration, error) {
	payload := availabilityRequest{
		ZIPCode:               settings.ZIPCode,
		ShowOnShelf:           true,
		LookupInStoreQuantity: true,
		OnlyBestBuyLocations:  true,
		PickupTypes:           []string{"UPS_ACCESS_POINT", "FEDEX_HAL"},
		Items: []requestItem{{
			SKU:                 settings.SKU,
			Quantity:            1,
			ItemSeqNumber:       "1",
			SelectedServices:    []string{},
			RequiredAccessories: []string{},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.bestbuy.com/productfulfillment/c/api/2.0/storeAvailability", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.bestbuy.com")
	req.Header.Set("Referer", productURL())
	res, err := m.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("best buy request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, retryAfter(res.Header.Get("Retry-After")), fmt.Errorf("best buy HTTP %d", res.StatusCode)
	}
	body, err = io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, 0, fmt.Errorf("best buy response unreadable or larger than 4 MiB")
	}
	var data availabilityResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, 0, fmt.Errorf("decode best buy response: %w", err)
	}
	if data.Pickup == nil || len(data.Pickup.Locations) == 0 || len(data.Pickup.Items) != 1 || data.Pickup.Items[0].SKU != settings.SKU || len(data.Pickup.Items[0].Locations) == 0 {
		return nil, 0, fmt.Errorf("missing or ambiguous requested SKU pickup data")
	}
	locations := make(map[string]location)
	for _, loc := range data.Pickup.Locations {
		if _, exists := locations[loc.ID]; loc.ID == "" || exists {
			return nil, 0, fmt.Errorf("missing or duplicate location ID")
		}
		locations[loc.ID] = loc
	}
	seen := make(map[string]bool)
	var stocks []stock
	for _, item := range data.Pickup.Items[0].Locations {
		loc, ok := locations[item.ID]
		if !ok || seen[item.ID] || loc.Distance == nil || *loc.Distance < 0 {
			return nil, 0, fmt.Errorf("missing or ambiguous store metadata")
		}
		seen[item.ID] = true
		if *loc.Distance > settings.RadiusMiles || (len(settings.StoreIDs) > 0 && !slices.Contains(settings.StoreIDs, item.ID)) {
			continue
		}
		if item.Availability.Quantity == nil || *item.Availability.Quantity < 0 || strings.TrimSpace(loc.Name) == "" {
			return nil, 0, fmt.Errorf("missing or invalid store pickup quantity or name")
		}
		stocks = append(stocks, stock{
			ID:       item.ID,
			Name:     loc.Name,
			Distance: *loc.Distance,
			Quantity: *item.Availability.Quantity,
		})
	}
	for _, id := range settings.StoreIDs {
		if !slices.ContainsFunc(stocks, func(s stock) bool { return s.ID == id }) {
			return nil, 0, fmt.Errorf("required store %s absent or outside radius", id)
		}
	}
	if len(stocks) == 0 || len(stocks) > 128 {
		return nil, 0, fmt.Errorf("expected 1–128 stores in search area; adjust radius or store filter")
	}
	slices.SortFunc(stocks, func(a, b stock) int { return strings.Compare(a.ID, b.ID) })

	return stocks, 0, nil
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 && seconds <= int64((1<<63-1)/time.Second) {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(date))
	}

	return 0
}

func stockEvent(s stock, old int) monitord.Event {
	title, color := "Pickup quantity changed", 0x0046BE
	if old == 0 && s.Quantity > 0 {
		title, color = "Pickup restock", 0x34C759
	} else if s.Quantity == 0 {
		title, color = "Pickup sold out", 0x808080
	}
	name := []rune(s.Name)
	if len(name) > 150 {
		name = name[:150]
	}

	return monitord.Event{
		Title:       title + " · Best Buy SKU " + settings.SKU,
		Description: string(name),
		URL:         productURL(),
		Color:       color,
		Fields: []monitord.EventField{
			{Name: "Store", Value: s.ID, Inline: true},
			{Name: "Pickup quantity", Value: fmt.Sprintf("%d → %d", old, s.Quantity), Inline: true},
			{Name: "Distance", Value: fmt.Sprintf("%.1f miles", s.Distance), Inline: true},
		},
		Footer: "Best Buy pickup • Availability is not a reservation",
	}
}
