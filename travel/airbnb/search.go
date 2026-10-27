package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type searchFilter struct {
	Name   string   `json:"filterName"`
	Values []string `json:"filterValues"`
}

type searchRequest struct {
	MetadataOnly bool           `json:"metadataOnly"`
	MaxMapItems  int            `json:"maxMapItems,omitempty"`
	Cursor       string         `json:"cursor,omitempty"`
	Filters      []searchFilter `json:"rawParams"`
	PageType     string         `json:"requestedPageType"`
	SearchType   string         `json:"searchType,omitempty"`
	Flags        []string       `json:"treatmentFlags"`
}

type searchVariables struct {
	Map  searchRequest `json:"staysMapSearchRequestV2"`
	List searchRequest `json:"staysSearchRequest"`
}

type searchCard struct {
	Listing struct {
		ID string `json:"id"`
	} `json:"demandStayListing"`
	Name struct {
		Text string `json:"localizedStringWithTranslationPreference"`
	} `json:"nameLocalized"`
	Pictures []struct {
		URL string `json:"picture"`
	} `json:"contextualPictures"`
}

type pagination struct {
	Cursors []string `json:"pageCursors"`
	Next    *string  `json:"nextPageCursor"`
}

type searchResponse struct {
	Errors []json.RawMessage `json:"errors"`
	Data   struct {
		Presentation struct {
			Search *struct {
				Results *struct {
					Cards      []searchCard `json:"searchResults"`
					Pagination *pagination  `json:"paginationInfo"`
				} `json:"results"`
			} `json:"staysSearch"`
		} `json:"presentation"`
	} `json:"data"`
}

func (s *source) bootstrap(ctx context.Context) (string, searchVariables, error) {
	var variables searchVariables
	page, err := s.get(ctx, s.config.searchURL(), "")
	if err != nil {
		return "", variables, err
	}
	key := keyPattern.FindSubmatch(page)
	if len(key) != 2 {
		return "", variables, fmt.Errorf("Airbnb anonymous API key absent from page")
	}
	raw, err := bootstrapVariables(page, "StaysSearch")
	if err != nil {
		return "", variables, err
	}
	if err := json.Unmarshal(raw, &variables); err != nil {
		return "", variables, fmt.Errorf("decode search variables: %w", err)
	}
	for _, request := range []searchRequest{variables.List, variables.Map} {
		if err := s.validateSearch(request); err != nil {
			return "", variables, err
		}
	}
	if err := s.discoverOperations(ctx, page, "StaysSearch"); err != nil {
		return "", variables, err
	}

	return string(key[1]), variables, nil
}

func (s *source) validateSearch(request searchRequest) error {
	c := s.config
	wanted := map[string]string{
		"adults": strconv.Itoa(c.Adults), "checkin": c.CheckIn, "checkout": c.CheckOut,
		"currency": c.Currency, "searchByMap": "true",
		"neLat": number(c.Area.North), "neLng": number(c.Area.East),
		"swLat": number(c.Area.South), "swLng": number(c.Area.West),
	}
	for name, minimum := range map[string]float64{"minBedrooms": c.MinBedrooms, "minBeds": c.MinBeds, "minBathrooms": c.MinBathrooms} {
		if minimum > 0 {
			wanted[name] = number(minimum)
		}
	}
	seen := make(map[string]bool)
	for _, filter := range request.Filters {
		if seen[filter.Name] {
			return fmt.Errorf("duplicate search filter %s", filter.Name)
		}
		seen[filter.Name] = true
		want, required := wanted[filter.Name]
		if !required {
			continue
		}
		if len(filter.Values) != 1 || !equalFilter(filter.Values[0], want) {
			return fmt.Errorf("Airbnb changed required search filter %s", filter.Name)
		}
		delete(wanted, filter.Name)
	}
	if len(wanted) != 0 || request.PageType != "STAYS_SEARCH" || request.MetadataOnly || request.Cursor != "" {
		return fmt.Errorf("Airbnb bootstrap omitted required search constraints or changed page schema")
	}

	return nil
}

func equalFilter(got, want string) bool {
	if got == want {
		return true
	}
	a, aErr := strconv.ParseFloat(got, 64)
	b, bErr := strconv.ParseFloat(want, 64)

	return aErr == nil && bErr == nil && a == b
}

func withoutDates(request searchRequest) searchRequest {
	request.Filters = slices.DeleteFunc(slices.Clone(request.Filters), func(filter searchFilter) bool {
		return filter.Name == "checkin" || filter.Name == "checkout"
	})

	return request
}

func (s *source) search(ctx context.Context, key string, variables searchVariables) (map[string]observation, error) {
	found := make(map[string]observation)
	var cursors []string
	for page := 0; page < s.config.MaxPages; page++ {
		if page > 0 {
			variables.List.Cursor = cursors[page]
		}
		raw, err := json.Marshal(variables)
		if err != nil {
			return nil, err
		}
		body, err := s.query(ctx, key, "StaysSearch", raw)
		if err != nil {
			return nil, err
		}
		var response searchResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("decode search response: %w", err)
		}
		search := response.Data.Presentation.Search
		if len(response.Errors) != 0 || search == nil || search.Results == nil || search.Results.Cards == nil || search.Results.Pagination == nil {
			return nil, fmt.Errorf("search returned errors or incomplete data")
		}
		result := search.Results
		if page == 0 {
			cursors = result.Pagination.Cursors
			if len(cursors) > s.config.MaxPages {
				return nil, fmt.Errorf("search requires %d pages, exceeding MaxPages=%d; narrow the area or filters", len(cursors), s.config.MaxPages)
			}
			for index, cursor := range cursors {
				if index > 0 && (cursor == "" || slices.Contains(cursors[:index], cursor)) {
					return nil, fmt.Errorf("search returned empty or repeated continuation cursor")
				}
			}
		} else if !slices.Equal(cursors, result.Pagination.Cursors) {
			return nil, fmt.Errorf("search pagination changed during traversal; refusing partial observation")
		}
		for _, card := range result.Cards {
			id, err := listingID(card.Listing.ID)
			if err != nil {
				return nil, err
			}
			item := observation{Name: card.Name.Text}
			if len(card.Pictures) > 0 {
				item.Image = card.Pictures[0].URL
			}
			found[id] = item
			if len(found) > s.config.MaxListings {
				return nil, fmt.Errorf("search exceeds MaxListings=%d; narrow the area or filters", s.config.MaxListings)
			}
		}
		if page+1 >= len(cursors) {
			if result.Pagination.Next != nil && *result.Pagination.Next != "" {
				return nil, fmt.Errorf("search has an unrepresented continuation cursor")
			}

			return found, nil
		}
	}

	return nil, fmt.Errorf("search pagination exhausted its bound")
}

func listingID(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !strings.HasPrefix(string(raw), "DemandStayListing:") {
		return "", fmt.Errorf("invalid Airbnb listing identity")
	}
	id := strings.TrimPrefix(string(raw), "DemandStayListing:")
	if id == "" || strings.Trim(id, "0123456789") != "" {
		return "", fmt.Errorf("Airbnb listing identity is not numeric")
	}

	return id, nil
}
