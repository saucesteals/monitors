package main

import (
	"context"
	"fmt"
	"github.com/saucesteals/monitord"
	"github.com/saucesteals/shop"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

func (m *monitor) fetch(ctx context.Context) ([]Observation, error) {
	found := map[string]shop.ProductSummary{}
	verificationFailures := 0
	for _, query := range settings.Queries {
		for page := 1; page <= settings.Pages; page++ {
			result, err := m.store.Search(ctx, &shop.SearchQuery{Query: query, Page: page, Sort: shop.SortNewest, MinPrice: &settings.MinCents, MaxPrice: &settings.MaxCents, Filters: map[string]string{"city": settings.City, "radius": settings.Radius, "days_since_listed": strconv.Itoa(settings.Days), "shipping": "false"}})
			if err != nil {
				return nil, fmt.Errorf("marketplace search: %w", err)
			}
			if result == nil {
				return nil, fmt.Errorf("missing search result")
			}
			slog.InfoContext(ctx, "marketplace query page", "query", query, "page", page, "products", len(result.Products), "warnings", result.Warnings)
			// Unverifiable listings are omitted by the SDK and remain eligible on
			// later scans. A rejected page must not discard other valid pages.
			for _, warning := range result.Warnings {
				if len(result.Products) == 0 && (strings.Contains(warning, "without a verifiable Marketplace city") || strings.Contains(warning, "incomplete Marketplace listing")) {
					verificationFailures++
					slog.WarnContext(ctx, "marketplace page lost to verification", "query", query, "page", page, "warning", warning)
				}
			}
			for _, p := range result.Products {
				if p.ID == "" || p.Title == "" {
					return nil, fmt.Errorf("incomplete listing")
				}
				found[p.ID] = p
			}
			if !result.HasMore {
				break
			}
			if page == settings.Pages {
				return nil, fmt.Errorf("marketplace query %q still has more results after %d pages; refusing a truncated scan", query, page)
			}
		}
	}
	if len(found) == 0 && verificationFailures > 0 {
		return nil, fmt.Errorf("incomplete marketplace feed: no verified listings and %d verification warnings", verificationFailures)
	}
	ids := make([]string, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Observation
	for _, id := range ids {
		p := found[id]
		// Relevance and price are requested upstream. Do not discard Facebook
		// results using title regexes or a second local relevance classifier.
		out = append(out, Observation{ID: id, Match: true, Event: listingEvent(p)})
	}
	return out, nil
}

func listingEvent(p shop.ProductSummary) monitord.Event {
	price := "Price not listed"
	if p.Price != nil {
		price = fmt.Sprintf("%d.%02d %s", p.Price.Amount/100, p.Price.Amount%100, p.Price.Currency)
		if p.Price.Currency == "USD" {
			price = fmt.Sprintf("$%d.%02d", p.Price.Amount/100, p.Price.Amount%100)
		}
	}
	location, _ := p.Attributes["location"].(string)
	if location == "" {
		location = settings.City
	}
	return monitord.Event{ID: "listing:" + p.ID, Title: p.Title, Description: "**New matching listing** · Facebook Marketplace", URL: p.URL, Image: p.ImageURL, Color: 0x71B89B, Fields: []monitord.EventField{{Name: "Price", Value: price, Inline: true}, {Name: "Location", Value: location, Inline: true}, {Name: "Pickup radius", Value: strings.ReplaceAll(settings.Radius, "mi", " mi"), Inline: true}}, Mentions: []string{}}
}
