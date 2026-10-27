package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
)

// Public web-client operation identity, not a credential. Revalidate on HTTP 409.
const availabilityQuery = "e3beaff77e6479354bac1363476378b05151ec8dbf2d6eafe386218ed0b68a27"

type queryVariables struct {
	RestaurantIDs                []int    `json:"restaurantIds"`
	Date                         string   `json:"date"`
	Time                         string   `json:"time"`
	PartySize                    int      `json:"partySize"`
	DatabaseRegion               string   `json:"databaseRegion"`
	OnlyPop                      bool     `json:"onlyPop"`
	RestaurantAvailabilityTokens []string `json:"restaurantAvailabilityTokens"`
	LoyaltyRedemptionTiers       []string `json:"loyaltyRedemptionTiers"`
	RequireTypes                 []string `json:"requireTypes"`
	PrivilegedAccess             []string `json:"privilegedAccess"`
	ForwardDays                  int      `json:"forwardDays"`
	ForwardMinutes               int      `json:"forwardMinutes"`
	BackwardMinutes              int      `json:"backwardMinutes"`
	ForwardTimeslots             int      `json:"forwardTimeslots"`
	BackwardTimeslots            int      `json:"backwardTimeslots"`
	CorrelationID                string   `json:"correlationId"`
	UseCBR                       bool     `json:"useCBR"`
}

func availabilityPayload(date string, party, minutes int) ([]byte, error) {
	request := struct {
		Operation  string         `json:"operationName"`
		Variables  queryVariables `json:"variables"`
		Extensions struct {
			PersistedQuery struct {
				Version int    `json:"version"`
				Hash    string `json:"sha256Hash"`
			} `json:"persistedQuery"`
		} `json:"extensions"`
	}{
		Operation: "RestaurantsAvailability",
		Variables: queryVariables{
			RestaurantIDs: []int{settings.RestaurantID}, Date: date, Time: settings.StartTime,
			PartySize: party, DatabaseRegion: "NA", RequireTypes: []string{"Standard"},
			RestaurantAvailabilityTokens: []string{}, LoyaltyRedemptionTiers: []string{}, PrivilegedAccess: []string{},
			ForwardMinutes: minutes, ForwardTimeslots: minutes/15 + 16,
		},
	}
	request.Extensions.PersistedQuery.Version = 1
	request.Extensions.PersistedQuery.Hash = availabilityQuery

	return json.Marshal(request)
}

type queryResponse struct {
	Data *struct {
		Availability []struct {
			RestaurantID int `json:"restaurantId"`
			Days         []struct {
				Offset *int `json:"dayOffset"`
				Slots  *[]struct {
					Available *bool  `json:"isAvailable"`
					Offset    *int   `json:"timeOffsetMinutes"`
					Type      string `json:"type"`
				} `json:"slots"`
			} `json:"availabilityDays"`
		} `json:"availability"`
	} `json:"data"`
	Errors []json.RawMessage `json:"errors"`
}

func (m *monitor) decode(body []byte, date string) ([]string, error) {
	var response queryResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode availability: %w", err)
	}
	if len(response.Errors) != 0 || response.Data == nil {
		return nil, fmt.Errorf("graphql returned errors or missing data")
	}
	// Offsets are wall-clock minutes, not elapsed time across a DST boundary.
	base, err := time.Parse("2006-01-02T15:04", date+"T"+settings.StartTime)
	if err != nil {
		return nil, err
	}
	slots, matched := []string{}, false
	for _, restaurant := range response.Data.Availability {
		if restaurant.RestaurantID != settings.RestaurantID {
			continue
		}
		for _, day := range restaurant.Days {
			if day.Offset == nil {
				return nil, fmt.Errorf("missing day offset")
			}
			if *day.Offset != 0 {
				continue
			}
			if matched || day.Slots == nil {
				return nil, fmt.Errorf("duplicate day or missing slots")
			}
			matched = true
			for _, slot := range *day.Slots {
				// Unavailable placeholders legitimately omit the remaining fields.
				if slot.Available != nil && !*slot.Available {
					continue
				}
				if slot.Available == nil || slot.Offset == nil || slot.Type == "" {
					return nil, fmt.Errorf("incomplete slot")
				}
				if slot.Type != "Standard" || *slot.Offset < 0 || *slot.Offset > m.minutes {
					continue
				}
				local := base.Add(time.Duration(*slot.Offset) * time.Minute).Format("2006-01-02T15:04")
				at, err := time.ParseInLocation("2006-01-02T15:04", local, m.zone)
				if err != nil || at.Format("2006-01-02T15:04") != local {
					return nil, fmt.Errorf("invalid restaurant-local slot")
				}
				if at.After(time.Now()) {
					slots = append(slots, at.Format("15:04"))
				}
			}
		}
	}
	if !matched {
		return nil, fmt.Errorf("requested restaurant/date missing")
	}
	slices.Sort(slots)

	return slices.Compact(slots), nil
}

func openingEvent(date string, party int, slots []string) monitord.Event {
	query := url.Values{
		"rid": {strconv.Itoa(settings.RestaurantID)}, "restref": {strconv.Itoa(settings.RestaurantID)},
		"covers": {strconv.Itoa(party)}, "datetime": {date + "T" + slots[0]},
	}

	return monitord.Event{
		Title:       settings.Label,
		Description: "**Tables opened up** · OpenTable\nChoose a time on the restaurant page to confirm availability.",
		URL:         settings.ProfileURL + "?" + query.Encode(),
		Color:       0xDA3743,
		Fields: []monitord.EventField{
			{Name: "Date", Value: date, Inline: true},
			{Name: "Party", Value: strconv.Itoa(party), Inline: true},
			{Name: "New times", Value: strings.Join(slots, " · ")},
		},
		Footer: "Restaurant-local time · " + settings.Timezone,
	}
}
