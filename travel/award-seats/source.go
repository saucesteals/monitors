package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/monitord"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type requestBody struct {
	Origins         []string `json:"origins"`
	Destinations    []string `json:"destinations"`
	Dates           []string `json:"dates"`
	NumADTs         int      `json:"numADTs"`
	NumINFs         int      `json:"numINFs"`
	NumCHDs         int      `json:"numCHDs"`
	FareView        string   `json:"fareView"`
	Onba            bool     `json:"onba"`
	Dnba            bool     `json:"dnba"`
	Discount        discount `json:"discount"`
	IsAlaska        bool     `json:"isAlaska"`
	IsMobileApp     bool     `json:"isMobileApp"`
	SliceID         int      `json:"sliceId"`
	BusinessRequest business `json:"businessRequest"`
	UmnrAgeGroup    string   `json:"umnrAgeGroup"`
	LockFare        bool     `json:"lockFare"`
	SessionID       string   `json:"sessionID"`
	SolutionIDs     []any    `json:"solutionIDs"`
	SolutionSetIDs  []any    `json:"solutionSetIDs"`
	QpxcVersion     string   `json:"qpxcVersion"`
	TrackingTags    []any    `json:"trackingTags"`
}

type discount struct {
	Code                         string `json:"code"`
	Status                       int    `json:"status"`
	ExpirationDate               string `json:"expirationDate"`
	Message                      string `json:"message"`
	Memo                         string `json:"memo"`
	Type                         int    `json:"type"`
	SearchContainsDiscountedFare bool   `json:"searchContainsDiscountedFare"`
	CampaignName                 string `json:"campaignName"`
	CampaignCode                 string `json:"campaignCode"`
	Distribution                 int    `json:"distribution"`
	Amount                       int    `json:"amount"`
	ValidationErrors             []any  `json:"validationErrors"`
	MaxPassengers                int    `json:"maxPassengers"`
}

type business struct {
	TravelerID           string `json:"TravelerId"`
	BusinessRequestType  int    `json:"BusinessRequestType"`
	CountryCode          string `json:"CountryCode"`
	StateCode            string `json:"StateCode"`
	ShowOnlySpecialFares bool   `json:"ShowOnlySpecialFares"`
}

type response struct {
	Rows []row `json:"rows"`
}

type upgradeInfo struct {
	FirstClassAvailable *bool    `json:"firstClassAvailable"`
	ApplicableAirports  []string `json:"applicableAirports"`
}

type row struct {
	UpgradeInfo []upgradeInfo       `json:"upgradeInfo"`
	Segments    []segment           `json:"segments"`
	Solutions   map[string]solution `json:"solutions"`
}

type segment struct {
	Aircraft          string   `json:"aircraft"`
	Amenities         []string `json:"amenities"`
	FirstAmenities    []string `json:"firstAmenities"`
	PublishingCarrier carrier  `json:"publishingCarrier"`
	DepartureStation  string   `json:"departureStation"`
	ArrivalStation    string   `json:"arrivalStation"`
	DepartureTime     string   `json:"departureTime"`
	ArrivalTime       string   `json:"arrivalTime"`
	Duration          int      `json:"duration"`
}

type carrier struct {
	CarrierCode  string `json:"carrierCode"`
	FlightNumber int    `json:"flightNumber"`
}

type solution struct {
	BookingCodes   []string `json:"bookingCodes"`
	AtmosPoints    int      `json:"atmosPoints"`
	GrandTotal     float64  `json:"grandTotal"`
	SeatsRemaining int      `json:"seatsRemaining"`
	Cabins         []string `json:"cabins"`
	FareBasisCodes []string `json:"fareBasisCodes"`
}

type Offer struct {
	Date, Route, Flights, Cabin, Departure, Arrival string
	Points, Seats                                   int
	Cash                                            float64
}

func (m *monitor) search(ctx context.Context, date string) (map[string]Offer, error) {
	payload, err := json.Marshal(requestBody{Origins: []string{strings.ToLower(settings.Origin)}, Destinations: []string{strings.ToLower(settings.Destination)}, Dates: []string{date}, NumADTs: settings.Passengers, FareView: "as_awards", SolutionIDs: []any{}, SolutionSetIDs: []any{}, TrackingTags: []any{}, Discount: discount{ExpirationDate: "2026-01-01T00:00:00Z", ValidationErrors: []any{}}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://www.alaskaair.com/search/api/flightresults", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Origin", "https://www.alaskaair.com")
	req.Header.Set("Referer", "https://www.alaskaair.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Cache-Control", "no-cache")
	res, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("award request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Alaska HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(b) > 8<<20 {
		return nil, fmt.Errorf("invalid award response size")
	}
	var result response
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if result.Rows == nil {
		return nil, fmt.Errorf("missing award rows")
	}
	offers := map[string]Offer{}
	for _, row := range result.Rows {
		if len(row.Segments) == 0 || row.Solutions == nil {
			return nil, fmt.Errorf("missing itinerary segments")
		}
		if len(row.Segments) > settings.MaxStops+1 {
			continue
		}
		if row.Segments[0].DepartureStation != settings.Origin || row.Segments[len(row.Segments)-1].ArrivalStation != settings.Destination {
			continue
		}
		var flights []string
		for _, s := range row.Segments {
			if s.DepartureTime == "" || s.PublishingCarrier.CarrierCode == "" {
				return nil, fmt.Errorf("incomplete itinerary")
			}
			flights = append(flights, fmt.Sprintf("%s %d", s.PublishingCarrier.CarrierCode, s.PublishingCarrier.FlightNumber))
		}
		codes := make([]string, 0, len(row.Solutions))
		for code := range row.Solutions {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			fare := row.Solutions[code]
			if fare.AtmosPoints <= 0 || fare.AtmosPoints > settings.MaxPoints || fare.SeatsRemaining < settings.Passengers || containsInt(settings.IgnoreSeatCounts, fare.SeatsRemaining) || len(fare.Cabins) != len(row.Segments) {
				continue
			}
			match := true
			for _, c := range fare.Cabins {
				if !contains(settings.Cabins, c) {
					match = false
				}
			}
			if !match {
				continue
			}
			o := Offer{Date: date, Route: settings.Origin + " → " + settings.Destination, Flights: strings.Join(flights, " · "), Cabin: strings.Join(fare.Cabins, " / "), Departure: row.Segments[0].DepartureTime, Arrival: row.Segments[len(row.Segments)-1].ArrivalTime, Points: fare.AtmosPoints, Seats: fare.SeatsRemaining, Cash: fare.GrandTotal}
			id := o.Flights + ":" + o.Departure + ":" + o.Cabin
			if old, ok := offers[id]; !ok || o.Points < old.Points || (o.Points == old.Points && o.Cash < old.Cash) {
				offers[id] = o
			}
		}
	}
	return offers, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func containsInt(values []int, value int) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func awardEvent(o Offer) monitord.Event {
	q := url.Values{"O": {settings.Origin}, "D": {settings.Destination}, "OD": {o.Date}, "A": {strconv.Itoa(settings.Passengers)}, "RT": {"false"}, "ShoppingMethod": {"onlineaward"}}
	date, _ := time.Parse("2006-01-02", o.Date)
	return monitord.Event{ID: "example:award", Title: o.Route + " · " + strings.ToLower(o.Cabin), Description: "**Award space opened** · Alaska Atmos\n" + o.Flights, URL: "https://www.alaskaair.com/search/results?" + q.Encode(), Color: 0x88B6DD, Fields: []monitord.EventField{{Name: "Date", Value: date.Format("Mon, Jan 2, 2006"), Inline: false}, {Name: "Award price", Value: fmt.Sprintf("%s + $%.2f", grouped(o.Points), o.Cash), Inline: true}, {Name: "Seats", Value: strconv.Itoa(o.Seats), Inline: true}, {Name: "Stops", Value: strconv.Itoa(strings.Count(o.Flights, " · ")), Inline: true}}, Footer: "Premium cabin on every segment", Mentions: []string{}}
}

func grouped(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
