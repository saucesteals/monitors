package main

import (
	"fmt"
	"time"
)

var settings = Config{
	City: "burlingtonct", Radius: "20mi", CityID: "109423279083274", Latitude: 41.7692, Longitude: -72.965, RadiusKM: 32,
	Queries:  []string{"firewood"},
	MinCents: 0, MaxCents: 100000000, Currency: "USD",
	Days: 1, Interval: 30 * time.Second,
}

type Config struct {
	City, Radius        string
	CityID              string
	Latitude, Longitude float64
	RadiusKM            int
	Queries             []string
	MinCents, MaxCents  int64
	Currency            string
	Days                int
	Interval            time.Duration
}

func sourceIdentity() any {
	return []any{"shop-coordinates-first-page-v1", settings.CityID, settings.Latitude, settings.Longitude, settings.RadiusKM, settings.City, settings.Radius, settings.Queries, settings.MinCents, settings.MaxCents, settings.Currency, settings.Days}
}

func validate() error {
	if settings.City == "" || settings.CityID == "" || settings.Latitude < -90 || settings.Latitude > 90 || settings.Longitude < -180 || settings.Longitude > 180 || settings.RadiusKM <= 0 || len(settings.Queries) == 0 || settings.Days < 1 || settings.Days > 365 || settings.Interval < 30*time.Second || settings.MinCents < 0 || settings.MaxCents < settings.MinCents {
		return fmt.Errorf("invalid search configuration")
	}
	return nil
}
