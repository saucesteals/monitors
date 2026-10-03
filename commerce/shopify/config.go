package main

import "time"

var settings = Config{
	Store: "https://kith.com", Label: "Kith", Collection: "mens-footwear", TitleContains: "J.L-A.L",
	Handles: []string{}, Sizes: []string{}, Events: []string{"new", "restock", "price-drop"},
	Currency: "USD", ImageMode: "image", Interval: 5 * time.Minute,
}

type Config struct {
	Store         string
	Label         string
	Collection    string // Optional Shopify collection handle; empty scans the storefront.
	TitleContains string
	Handles       []string // Empty matches every product; otherwise exact product handles.
	Sizes         []string // Empty matches every variant; otherwise exact title/option values.
	Events        []string // new, restock, price-drop
	Currency      string   // Match the storefront's market currency.
	ImageMode     string   // image, thumbnail, none
	Interval      time.Duration
}

// Presentation and polling changes must not reset source progress.
func sourceIdentity() any {
	return []any{settings.Store, settings.Collection, settings.TitleContains, settings.Handles, settings.Sizes, settings.Events}
}
