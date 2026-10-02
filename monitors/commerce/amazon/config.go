package main

import (
	"fmt"
	"regexp"
	"time"
)

var settings = Config{ASINs: []string{"B0F3GWXLTS"}, SellerID: "ATVPDKIKX0DER", ShipsFrom: []string{"Amazon", "Amazon.com"}, MaxCents: 49900, Currency: "USD", Condition: "", ShopConfig: "", MaxPages: 5, Interval: 2 * time.Minute}

type Config struct {
	ASINs               []string
	SellerID            string   // Empty permits any seller.
	ShipsFrom           []string // Exact names; empty permits any shipper.
	MaxCents            int64    // Zero removes the ceiling.
	Currency, Condition string   // Empty condition accepts unclassified offers too.
	ShopConfig          string   // Empty uses Shop's normal configuration directory.
	MaxPages            int
	Interval            time.Duration
}

func validate() error {
	if len(settings.ASINs) == 0 || settings.MaxPages < 1 || settings.MaxPages > 20 || settings.MaxCents < 0 || settings.Interval < time.Minute {
		return fmt.Errorf("invalid Amazon configuration")
	}
	for _, id := range settings.ASINs {
		if !regexp.MustCompile(`^[A-Z0-9]{10}$`).MatchString(id) {
			return fmt.Errorf("invalid ASIN")
		}
	}
	return nil
}

func sourceIdentity() any {
	return []any{settings.SellerID, settings.ShipsFrom, settings.MaxCents, settings.Currency, settings.Condition, settings.ShopConfig, settings.MaxPages}
}
