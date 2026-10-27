package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/saucesteals/shop/tracking"
)

type config struct {
	ShopConfig       string
	Carriers         []tracking.Carrier
	Merchants        []string
	LabelContains    string
	AllDeliveries    bool
	DeliveredSince   string
	Interval         time.Duration
	LookupTimeout    time.Duration
	MaxRefreshes     int
	MaxObservations  int
	ETAMinimumChange time.Duration
}

var settings = config{
	ShopConfig:       "", // Empty uses Shop's default configuration directory.
	Interval:         10 * time.Minute,
	LookupTimeout:    35 * time.Second,
	MaxRefreshes:     8,
	MaxObservations:  50,
	ETAMinimumChange: time.Hour,
}

func validate() (tracking.Filter, error) {
	filter := tracking.Filter{All: settings.AllDeliveries}
	if settings.DeliveredSince != "" {
		day, err := time.Parse(time.DateOnly, settings.DeliveredSince)
		if err != nil {
			return filter, fmt.Errorf("delivered-since must be YYYY-MM-DD")
		}
		filter.DeliveredSince = day
	}
	if err := filter.Validate(); err != nil {
		return filter, err
	}
	if settings.Interval < time.Minute || settings.Interval > 24*time.Hour {
		return filter, fmt.Errorf("interval must be between one minute and 24 hours")
	}
	if settings.LookupTimeout < time.Second || settings.LookupTimeout > time.Minute {
		return filter, fmt.Errorf("lookup timeout must be between one second and one minute")
	}
	if settings.MaxRefreshes < 1 || settings.MaxRefreshes > 64 || settings.MaxObservations < 1 || settings.MaxObservations > 256 {
		return filter, fmt.Errorf("set 1–64 refreshes and 1–256 saved observations per poll")
	}
	if settings.ETAMinimumChange < 0 || settings.ETAMinimumChange > 24*time.Hour {
		return filter, fmt.Errorf("ETA minimum change must be between zero and 24 hours")
	}
	for _, carrier := range settings.Carriers {
		switch carrier {
		case tracking.UPS, tracking.USPS, tracking.FedEx, tracking.GOFO, tracking.Yanwen:
		default:
			return filter, fmt.Errorf("unsupported carrier filter %q", carrier)
		}
	}
	for _, merchant := range settings.Merchants {
		if strings.TrimSpace(merchant) == "" {
			return filter, fmt.Errorf("merchant filters must not be blank")
		}
	}

	return filter, nil
}

func selected(shipment tracking.Shipment) bool {
	if len(settings.Carriers) > 0 {
		match := false
		for _, carrier := range settings.Carriers {
			match = match || tracking.DetectCarrier(shipment.TrackingNumber) == carrier
		}
		if !match {
			return false
		}
	}
	if len(settings.Merchants) > 0 {
		match := false
		for _, merchant := range settings.Merchants {
			match = match || strings.EqualFold(strings.TrimSpace(shipment.Merchant), strings.TrimSpace(merchant))
		}
		if !match {
			return false
		}
	}

	return strings.Contains(strings.ToLower(shipment.Label), strings.ToLower(strings.TrimSpace(settings.LabelContains)))
}
