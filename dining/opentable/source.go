package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/monitord"
)

// OpenTable's web-client persisted query; not a credential. If the website
// retires it, fail without altering observations until the adapter is updated.
const availabilityQuery = "cbcf4838a9b399f742e3741785df64560a826d8d3cc2828aa01ab09a8455e29e"

var csrfPattern = regexp.MustCompile(`(?:__CSRF_TOKEN__|"__CSRF_TOKEN__"|"csrfToken")\s*[:=]\s*("(?:[^"\\]|\\.)*")`)

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

func correlationID() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func profileURL() string {
	return "https://www.opentable.com/r/" + settings.Slug
}

func (m *monitor) bootstrap(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	body, status, err := m.read(req)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("profile HTTP %d", status)
	}
	match := csrfPattern.FindSubmatch(body)
	if len(match) != 2 || json.Unmarshal(match[1], &m.csrf) != nil || m.csrf == "" {
		return fmt.Errorf("profile did not supply a CSRF token")
	}

	return nil
}

func (m *monitor) read(req *http.Request) ([]byte, int, error) {
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	res, err := m.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("opentable request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if len(body) > 8<<20 {
		return nil, res.StatusCode, fmt.Errorf("response exceeds 8 MiB")
	}

	return body, res.StatusCode, nil
}

func (m *monitor) fetch(ctx context.Context, date string) ([]string, error) {
	if m.csrf == "" {
		if err := m.bootstrap(ctx); err != nil {
			return nil, err
		}
	}
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
			RestaurantIDs:                []int{settings.RestaurantID},
			Date:                         date,
			Time:                         settings.StartTime,
			PartySize:                    settings.PartySize,
			DatabaseRegion:               "NA",
			RestaurantAvailabilityTokens: []string{},
			LoyaltyRedemptionTiers:       []string{},
			RequireTypes:                 []string{"Standard"},
			PrivilegedAccess:             []string{},
			ForwardMinutes:               m.minutes,
			ForwardTimeslots:             m.minutes/15 + 16,
			CorrelationID:                correlationID(),
		},
	}
	request.Extensions.PersistedQuery.Version = 1
	request.Extensions.PersistedQuery.Hash = availabilityQuery
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	// Refresh an expired anonymous session once; do not spin on access denial.
	for attempt := range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.opentable.com/dapi/fe/gql?optype=query&opname=RestaurantsAvailability", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Origin", "https://www.opentable.com")
		req.Header.Set("Referer", profileURL())
		req.Header.Set("Ot-Page-Type", "restprofilepage")
		req.Header.Set("Ot-Page-Group", "rest-profile")
		req.Header.Set("X-CSRF-Token", m.csrf)
		req.Header.Set("X-Query-Timeout", "5500")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		body, status, err := m.read(req)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			m.csrf = ""
			if attempt == 0 {
				if err := m.bootstrap(ctx); err != nil {
					return nil, err
				}
				continue
			}
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("availability HTTP %d", status)
		}

		return m.decode(body, date)
	}

	return nil, fmt.Errorf("session refresh failed")
}

func (m *monitor) decode(body []byte, date string) ([]string, error) {
	var response queryResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode availability: %w", err)
	}
	if len(response.Errors) != 0 || response.Data == nil {
		return nil, fmt.Errorf("graphql returned errors or missing data; observation preserved")
	}
	// Offsets describe wall-clock minutes, not elapsed time across DST changes.
	base, err := time.Parse("2006-01-02T15:04", date+"T"+settings.StartTime)
	if err != nil {
		return nil, err
	}
	slots := []string{}
	matched := false
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
				if slot.Available == nil || slot.Offset == nil || slot.Type == "" {
					return nil, fmt.Errorf("incomplete slot")
				}
				if !*slot.Available || slot.Type != "Standard" || *slot.Offset < 0 || *slot.Offset > m.minutes {
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
		return nil, fmt.Errorf("requested restaurant/date missing; observation preserved")
	}
	slices.Sort(slots)

	return slices.Compact(slots), nil
}

func openingEvent(date string, slots []string) monitord.Event {
	query := url.Values{
		"rid":      {strconv.Itoa(settings.RestaurantID)},
		"restref":  {strconv.Itoa(settings.RestaurantID)},
		"covers":   {strconv.Itoa(settings.PartySize)},
		"datetime": {date + "T" + slots[0]},
	}

	return monitord.Event{
		Title:       settings.Label,
		Description: "**Tables opened up** · OpenTable\nChoose a time on the restaurant page to confirm availability.",
		URL:         profileURL() + "?" + query.Encode(),
		Color:       0xDA3743,
		Fields: []monitord.EventField{
			{Name: "Date", Value: date, Inline: true},
			{Name: "Party", Value: strconv.Itoa(settings.PartySize), Inline: true},
			{Name: "New times", Value: strings.Join(slots, " · ")},
		},
		Footer:   "Restaurant-local time · " + settings.Timezone,
		Mentions: []string{},
	}
}
