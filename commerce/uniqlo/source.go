package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type variant struct {
	ID    string `json:"l2Id"`
	Color struct {
		Code string `json:"displayCode"`
	} `json:"color"`
	Size struct {
		Code string `json:"displayCode"`
	} `json:"size"`
}

type stock struct {
	Status   string `json:"statusCode"`
	Quantity *int   `json:"quantity"`
}

type response struct {
	Status string `json:"status"`
	Result struct {
		Variants []variant        `json:"l2s"`
		Stocks   map[string]stock `json:"stocks"`
	} `json:"result"`
}

func productURL(selected selection) string {
	query := url.Values{
		"colorDisplayCode": {selected.Color},
		"sizeDisplayCode":  {selected.Size},
	}

	return fmt.Sprintf("https://www.uniqlo.com/%s/%s/products/%s/%s?%s", settings.Country, settings.Language, settings.Product, settings.PriceGroup, query.Encode())
}

func (m *monitor) fetch(ctx context.Context) (map[string]bool, time.Duration, error) {
	query := url.Values{
		"withPrices":           {"true"},
		"withStocks":           {"true"},
		"includePreviousPrice": {"false"},
		"withMemberPricing":    {"true"},
		"httpFailure":          {"true"},
	}
	endpoint := fmt.Sprintf("https://www.uniqlo.com/%s/api/commerce/v5/%s/products/%s/price-groups/%s/l2s?%s", settings.Country, settings.Language, settings.Product, settings.PriceGroup, query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("uniqlo stock request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, retryAfter(res.Header.Get("Retry-After")), fmt.Errorf("uniqlo HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, 0, fmt.Errorf("uniqlo response unreadable or larger than 4 MiB")
	}
	var payload response
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, fmt.Errorf("decode uniqlo response: %w", err)
	}
	if payload.Status != "ok" || len(payload.Result.Variants) == 0 {
		return nil, 0, fmt.Errorf("uniqlo returned an application error or no variants")
	}
	wanted := make(map[string]bool, len(settings.Variants))
	for _, selected := range settings.Variants {
		wanted[selected.key()] = true
	}
	available := make(map[string]bool, len(wanted))
	ids := make(map[string]bool)
	for _, item := range payload.Result.Variants {
		key := (selection{Color: item.Color.Code, Size: item.Size.Code}).key()
		if !wanted[key] {
			continue
		}
		if _, duplicate := available[key]; duplicate || item.ID == "" || ids[item.ID] {
			return nil, 0, fmt.Errorf("missing or ambiguous variant identity for %s", key)
		}
		ids[item.ID] = true
		inventory, ok := payload.Result.Stocks[item.ID]
		if !ok || inventory.Quantity == nil || *inventory.Quantity < 0 || strings.TrimSpace(inventory.Status) == "" {
			return nil, 0, fmt.Errorf("missing or invalid inventory for %s", key)
		}
		// Both fields must permit stock. Status alone is not a quantity guarantee.
		available[key] = *inventory.Quantity > 0 && inventory.Status != "STOCK_OUT"
	}
	for key := range wanted {
		if _, ok := available[key]; !ok {
			return nil, 0, fmt.Errorf("requested color/size %s absent from response", key)
		}
	}

	return available, 0, nil
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
