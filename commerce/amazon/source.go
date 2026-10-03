package main

import (
	"context"
	"fmt"
	"github.com/saucesteals/monitord"
	"github.com/saucesteals/shop"
	"strings"
)

func (m *monitor) findOffer(ctx context.Context, asin string) (*shop.Offer, error) {
	var best *shop.Offer
	for page := 1; page <= settings.MaxPages; page++ {
		result, err := m.store.Offers(ctx, asin, &shop.OffersQuery{Page: page, PageSize: 10})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("missing offer response")
		}
		for _, o := range result.Offers {
			if eligible(o) && (best == nil || o.Price.Amount < best.Price.Amount) {
				copy := o
				best = &copy
			}
		}
		if !result.HasMore {
			return best, nil
		}
	}
	return nil, fmt.Errorf("offer scan exceeded MaxPages; increase the bound")
}

func eligible(o shop.Offer) bool {
	if o.Availability.Status != shop.AvailabilityInStock && o.Availability.Status != shop.AvailabilityLowStock {
		return false
	}
	if o.Price.Currency != settings.Currency || o.Price.Amount <= 0 || (settings.MaxCents > 0 && o.Price.Amount > settings.MaxCents) {
		return false
	}
	if settings.SellerID != "" && o.Seller.ID != settings.SellerID {
		return false
	}
	if settings.Condition != "" && string(o.Condition) != settings.Condition {
		return false
	}
	if len(settings.ShipsFrom) > 0 {
		if o.Shipping == nil {
			return false
		}
		match := false
		for _, name := range settings.ShipsFrom {
			match = match || strings.EqualFold(strings.TrimSpace(o.Shipping.From), name)
		}
		if !match {
			return false
		}
	}
	return true
}

func offerEvent(p shop.Product, o shop.Offer, kind string) monitord.Event {
	fields := []monitord.EventField{{Name: "Price", Value: money(o.Price), Inline: true}, {Name: "Sold by", Value: o.Seller.Name, Inline: true}}
	if o.Shipping != nil && o.Shipping.From != "" {
		fields = append(fields, monitord.EventField{Name: "Ships from", Value: o.Shipping.From, Inline: true})
	}
	e := monitord.Event{ID: "example:" + p.ID, Title: p.Title, Description: "**" + kind + "** · Amazon", URL: "https://www.amazon.com/dp/" + p.ID, Color: 0xD9B875, Fields: fields, Mentions: []string{}}
	if len(p.Images) > 0 {
		e.Image = p.Images[0].URL
	}
	return e
}

func money(p shop.Money) string {
	if p.Currency == "USD" {
		return fmt.Sprintf("$%d.%02d", p.Amount/100, p.Amount%100)
	}
	return fmt.Sprintf("%d.%02d %s", p.Amount/100, p.Amount%100, p.Currency)
}
