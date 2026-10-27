package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type result struct {
	observation
	Price string
}

type stayDates struct {
	Start string `json:"startDate"`
	End   string `json:"endDate"`
}

type quoteVariables struct {
	ID     string    `json:"id"`
	Dates  stayDates `json:"dateRange"`
	Guests struct {
		Adults int `json:"numberOfAdults"`
	} `json:"guestCounts"`
	Calendar     bool      `json:"includePdpMigrationBookItCalendarSheetFragment"`
	Footer       bool      `json:"includePdpMigrationBookItFloatingFooterFragment"`
	Navigation   bool      `json:"includePdpMigrationBookItNavFragment"`
	Sidebar      bool      `json:"includePdpMigrationBookItSidebarFragment"`
	Gallery      bool      `json:"includeGalleryBookItCtaFragment"`
	Tips         bool      `json:"includeOverviewMerchandisingTipsFragment"`
	Heatmap      bool      `json:"includeStaysPdpPriceHeatmapFragment"`
	HeatmapDates stayDates `json:"priceHeatmapDateRange"`
}

type quoteResponse struct {
	Errors []json.RawMessage `json:"errors"`
	Data   struct {
		Node *struct {
			ID           *string `json:"id"`
			Presentation struct {
				Quote *struct {
					Availability *struct {
						Available *bool `json:"isAvailable"`
					} `json:"availability"`
					Price *struct {
						Explanation struct {
							Groups []struct {
								Items []struct {
									Description string `json:"description"`
									Price       string `json:"priceString"`
								} `json:"items"`
							} `json:"priceDetails"`
						} `json:"explanationData"`
					} `json:"productItemDetail"`
				} `json:"bookIt"`
			} `json:"pdpPresentation"`
		} `json:"node"`
	} `json:"data"`
}

type detailResponse struct {
	Errors []json.RawMessage `json:"errors"`
	Data   struct {
		Node *struct {
			ID       string `json:"id"`
			Location *struct {
				Coordinate *struct {
					Latitude  *float64 `json:"latitude"`
					Longitude *float64 `json:"longitude"`
				} `json:"coordinate"`
			} `json:"location"`
			Presentation struct {
				Overview *struct {
					Items []string `json:"items"`
				} `json:"overview"`
			} `json:"pdpPresentation"`
		} `json:"node"`
	} `json:"data"`
}

func (s *source) observe(ctx context.Context, previous map[string]observation) (map[string]result, error) {
	key, variables, err := s.bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make(map[string]observation, len(previous))
	for id, item := range previous {
		candidates[id] = item
	}
	// Date-free discovery can see currently blocked inventory. Neither search
	// is exhaustive: Airbnb ranking still limits what is returned.
	for _, undated := range []bool{false, true} {
		query := variables
		if undated {
			query.List = withoutDates(query.List)
			query.Map = withoutDates(query.Map)
		}
		found, err := s.search(ctx, key, query)
		if err != nil {
			return nil, err
		}
		for id, item := range found {
			candidates[id] = item
		}
		if len(candidates) > s.config.MaxListings {
			return nil, fmt.Errorf("retained candidate union exceeds MaxListings=%d; no listings evicted", s.config.MaxListings)
		}
	}
	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		if id == "" || strings.Trim(id, "0123456789") != "" {
			return nil, fmt.Errorf("invalid listing ID in checkpoint")
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	current := make(map[string]result, len(ids))
	for _, id := range ids {
		available, price, err := s.quote(ctx, key, id)
		if err != nil {
			return nil, fmt.Errorf("listing %s quote: %w", id, err)
		}
		if available {
			// Search can broaden its area and old candidates can change. The
			// current listing details, not ranking membership, decide eligibility.
			available, err = s.matches(ctx, key, id)
			if err != nil {
				return nil, fmt.Errorf("listing %s details: %w", id, err)
			}
		}
		item := candidates[id]
		item.Available = available
		current[id] = result{
			observation: item,
			Price:       price,
		}
	}

	return current, nil
}

func (s *source) quote(ctx context.Context, key, id string) (bool, string, error) {
	const operation = "StaysPdpBookItQuery"
	if s.operations[operation] == "" {
		page, err := s.get(ctx, s.config.listingURL(id), "")
		if err != nil {
			return false, "", err
		}
		if err := s.discoverOperations(ctx, page, operation, "StaysPdpSections"); err != nil {
			return false, "", err
		}
	}
	dates := stayDates{
		Start: s.config.CheckIn,
		End:   s.config.CheckOut,
	}
	variables := quoteVariables{
		ID:           base64.StdEncoding.EncodeToString([]byte("DemandStayListing:" + id)),
		Dates:        dates,
		Gallery:      true,
		Tips:         true,
		HeatmapDates: dates,
	}
	variables.Guests.Adults = s.config.Adults
	raw, err := json.Marshal(variables)
	if err != nil {
		return false, "", err
	}
	body, err := s.query(ctx, key, operation, raw)
	if err != nil {
		return false, "", err
	}
	var response quoteResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false, "", fmt.Errorf("decode quote: %w", err)
	}
	node := response.Data.Node
	if len(response.Errors) != 0 || node == nil || node.Presentation.Quote == nil {
		return false, "", fmt.Errorf("quote returned errors or no listing")
	}
	if node.ID != nil && *node.ID != variables.ID {
		return false, "", fmt.Errorf("quote returned a different listing identity")
	}
	quote := node.Presentation.Quote
	if quote.Availability == nil || quote.Availability.Available == nil {
		return false, "", fmt.Errorf("quote omitted its availability verdict")
	}
	var lines []string
	if quote.Price != nil {
		for _, group := range quote.Price.Explanation.Groups {
			for _, item := range group.Items {
				if item.Price != "" {
					lines = append(lines, strings.TrimSpace(item.Description+": "+item.Price))
				}
			}
		}
	}
	price := strings.Join(lines, "\n")
	if price == "" {
		price = "Airbnb did not supply price details"
	}
	// Bound presentation independently of the source verdict.
	if len([]rune(price)) > 1000 {
		price = string([]rune(price)[:997]) + "…"
	}

	return *quote.Availability.Available, price, nil
}

func (s *source) matches(ctx context.Context, key, id string) (bool, error) {
	const operation = "StaysPdpSections"
	page, err := s.get(ctx, s.config.listingURL(id), "")
	if err != nil {
		return false, err
	}
	variables, err := bootstrapVariables(page, operation)
	if err != nil {
		return false, err
	}
	if err := s.discoverOperations(ctx, page, operation); err != nil {
		return false, err
	}
	body, err := s.query(ctx, key, operation, variables)
	if err != nil {
		return false, err
	}
	var response detailResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false, fmt.Errorf("decode listing details: %w", err)
	}
	node := response.Data.Node
	if len(response.Errors) != 0 || node == nil || node.Location == nil || node.Location.Coordinate == nil {
		return false, fmt.Errorf("listing details returned errors or omitted coordinates")
	}
	returnedID, err := listingID(node.ID)
	if err != nil || returnedID != id {
		return false, fmt.Errorf("listing details returned a different or invalid identity")
	}
	point := node.Location.Coordinate
	if point.Latitude == nil || point.Longitude == nil || math.Abs(*point.Latitude) > 90 || math.Abs(*point.Longitude) > 180 {
		return false, fmt.Errorf("listing coordinates are missing or invalid")
	}
	a := s.config.Area
	if *point.Latitude > a.North || *point.Latitude < a.South || *point.Longitude > a.East || *point.Longitude < a.West {
		return false, nil
	}
	minima := map[string]float64{
		"bedroom": s.config.MinBedrooms,
		"bed":     s.config.MinBeds,
		"bath":    s.config.MinBathrooms,
	}
	counts := make(map[string]float64)
	if node.Presentation.Overview != nil {
		for _, item := range node.Presentation.Overview.Items {
			for _, match := range roomPattern.FindAllStringSubmatch(item, -1) {
				count, err := strconv.ParseFloat(match[1], 64)
				if err != nil {
					return false, fmt.Errorf("invalid listing room count")
				}
				room := strings.ToLower(match[2])
				if strings.HasPrefix(room, "bath") {
					room = "bath"
				} else {
					room = strings.TrimSuffix(room, "s")
				}
				counts[room] = count
			}
		}
	}
	for room, minimum := range minima {
		if minimum == 0 {
			continue
		}
		count, present := counts[room]
		if !present {
			return false, fmt.Errorf("listing details omitted a required %s count", room)
		}
		if count < minimum {
			return false, nil
		}
	}

	return true, nil
}

var roomPattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s+(?:shared\s+|private\s+)?(bedrooms?|beds?|baths?|bathrooms?)\b`)
