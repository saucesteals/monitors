package main

import "time"

var settings = Config{
	Shop: "fourseasonshotel-kyoto-sushionodera", Label: "Sushi Ginza Onodera · Kyoto",
	Timezone: "Asia/Tokyo", Times: []string{"2026-12-11T18:00", "2026-12-11T19:30"}, Adults: 2,
	ServiceCategory: "", MenuID: "6943ecd1ba31f5c7efe0a180", GroupOrder: true,
	Image:    "https://cdn3.tablecheck.com/menu_items/6943ecd1ba31f5c7efe0a180/images/xl/fba6c4df.jpg?1785905870",
	Interval: 2 * time.Minute,
}

type Config struct {
	Shop, Label, Timezone     string
	Times                     []string // Exact restaurant-local date/times: YYYY-MM-DDTHH:MM.
	Adults, Children, Infants int
	ServiceCategory, MenuID   string // Copy identifiers from that shop's reservation form.
	GroupOrder                bool   // Match the form's menu quantity/group-order semantics.
	Image                     string // Optional restaurant/menu photograph.
	Interval                  time.Duration
}

func sourceIdentity() any {
	return []any{settings.Shop, settings.Timezone, settings.Times, settings.Adults, settings.Children, settings.Infants, settings.ServiceCategory, settings.MenuID, settings.GroupOrder}
}
