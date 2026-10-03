package main

import (
	"context"
	"fmt"
	"github.com/saucesteals/monitord"
	"github.com/saucesteals/shop"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func (m *monitor) fetch(ctx context.Context) ([]Observation, error) {
	include := regexp.MustCompile(settings.Include)
	exclude := regexp.MustCompile(settings.Exclude)
	found := map[string]shop.ProductSummary{}
	for _, query := range settings.Queries {
		for page := 1; page <= settings.Pages; page++ {
			result, err := m.store.Search(ctx, &shop.SearchQuery{Query: query, Page: page, Sort: shop.SortNewest, MinPrice: &settings.MinCents, MaxPrice: &settings.MaxCents, Filters: map[string]string{"city": settings.City, "radius": settings.Radius, "days_since_listed": strconv.Itoa(settings.Days), "shipping": "false"}})
			if err != nil {
				return nil, fmt.Errorf("marketplace search: %w", err)
			}
			if result == nil {
				return nil, fmt.Errorf("missing search result")
			}
			// Do not silently advance a baseline when location filtering discarded the feed.
			for _, warning := range result.Warnings {
				if strings.Contains(warning, "were removed") {
					return nil, fmt.Errorf("incomplete marketplace feed: %s", warning)
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
		}
	}
	ids := make([]string, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Observation
	for _, id := range ids {
		p := found[id]
		match := p.Price != nil && p.Price.Currency == settings.Currency && p.Price.Amount >= settings.MinCents && p.Price.Amount <= settings.MaxCents && include.MatchString(p.Title) && (settings.Exclude == "" || !exclude.MatchString(p.Title))
		out = append(out, Observation{ID: id, Match: match, Event: listingEvent(p)})
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
