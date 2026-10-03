package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/saucesteals/monitord"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Variant struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Option1   string `json:"option1"`
	Option2   string `json:"option2"`
	Option3   string `json:"option3"`
	Price     string `json:"price"`
	Available *bool  `json:"available"`
}

type Product struct {
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	Handle   string    `json:"handle"`
	Variants []Variant `json:"variants"`
	Images   []struct {
		URL string `json:"src"`
	} `json:"images"`
}

type ProductCheckpoint struct {
	Product  Product `json:"product"`
	Sequence uint64  `json:"sequence"`
}

func validate() error {
	u, e := url.Parse(settings.Store)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("store must be an HTTPS origin")
	}
	if strings.ContainsAny(settings.Collection, "/?#") {
		return fmt.Errorf("collection must be a handle")
	}
	if len(settings.Currency) != 3 {
		return fmt.Errorf("currency must be a three-letter code")
	}
	if settings.ImageMode != "image" && settings.ImageMode != "thumbnail" && settings.ImageMode != "none" {
		return fmt.Errorf("invalid image mode")
	}
	if len(settings.Events) == 0 {
		return fmt.Errorf("choose at least one event")
	}
	for _, e := range settings.Events {
		if e != "new" && e != "restock" && e != "price-drop" {
			return fmt.Errorf("unknown event %q", e)
		}
	}
	return nil
}

func price(raw string) (int64, error) {
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "+") {
		return 0, fmt.Errorf("invalid price")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || len(parts[0]) == 0 {
		return 0, fmt.Errorf("invalid price")
	}
	whole, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || whole < 0 || whole > 1e12 {
		return 0, fmt.Errorf("invalid price")
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("invalid price precision")
		}
		fraction = (parts[1] + "00")[:2]
	}
	cents, e := strconv.ParseInt(fraction, 10, 64)
	if e != nil || cents < 0 {
		return 0, fmt.Errorf("invalid price")
	}
	return whole*100 + cents, nil
}

func (m *monitor) fetchProducts(ctx context.Context) ([]Product, error) {
	endpoint := strings.TrimRight(settings.Store, "/")
	if settings.Collection != "" {
		endpoint += "/collections/" + url.PathEscape(settings.Collection)
	}
	var all []Product
	seen := map[int64]bool{}
	for page := 1; page <= 100; page++ {
		b, e := m.get(ctx, fmt.Sprintf("%s/products.json?limit=250&page=%d", endpoint, page))
		if e != nil {
			return nil, e
		}
		var body struct {
			Products *[]Product `json:"products"`
		}
		if e = json.Unmarshal(b, &body); e != nil || body.Products == nil {
			return nil, fmt.Errorf("missing product array")
		}
		for _, p := range *body.Products {
			if p.ID == 0 || p.Title == "" || p.Handle == "" || len(p.Variants) == 0 || seen[p.ID] {
				return nil, fmt.Errorf("incomplete or repeated catalog product")
			}
			seen[p.ID] = true
			variants := map[int64]bool{}
			for _, v := range p.Variants {
				if v.ID == 0 || v.Available == nil || variants[v.ID] {
					return nil, fmt.Errorf("incomplete or duplicate variant")
				}
				variants[v.ID] = true
				if _, e = price(v.Price); e != nil {
					return nil, e
				}
			}
			all = append(all, p)
		}
		if len(*body.Products) < 250 {
			if len(all) == 0 {
				return nil, fmt.Errorf("empty catalog; baseline preserved")
			}
			sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
			return all, nil
		}
	}
	return nil, fmt.Errorf("catalog exceeds pagination limit")
}

func enabled(kind string) bool {
	for _, e := range settings.Events {
		if e == kind {
			return true
		}
	}
	return false
}

func productMatches(p Product) bool {
	if len(selected(p)) == 0 {
		return false
	}
	if !matches(p.Title, settings.TitleContains) {
		return false
	}
	if len(settings.Handles) == 0 {
		return true
	}
	for _, h := range settings.Handles {
		if h == p.Handle {
			return true
		}
	}
	return false
}

func selected(p Product) []Variant {
	var out []Variant
	for _, v := range p.Variants {
		ok := len(settings.Sizes) == 0
		for _, want := range settings.Sizes {
			for _, option := range []string{v.Title, v.Option1, v.Option2, v.Option3} {
				if strings.EqualFold(want, option) {
					ok = true
				}
			}
		}
		if ok {
			out = append(out, v)
		}
	}
	return out
}

func changes(old, p Product, known bool) []monitord.Event {
	if !known {
		return nil
	}
	previous := map[int64]Variant{}
	for _, v := range old.Variants {
		previous[v.ID] = v
	}
	var restocked, dropped, before []Variant
	for _, v := range selected(p) {
		prior, ok := previous[v.ID]
		if !ok {
			continue
		}
		if !*prior.Available && *v.Available {
			restocked = append(restocked, v)
		}
		oldPrice, _ := price(prior.Price)
		newPrice, _ := price(v.Price)
		if newPrice < oldPrice {
			dropped = append(dropped, v)
			before = append(before, prior)
		}
	}
	var out []monitord.Event
	if enabled("restock") && len(restocked) > 0 {
		out = append(out, productEvent("Back in stock", p, restocked, nil))
	}
	if enabled("price-drop") && len(dropped) > 0 {
		out = append(out, productEvent("Price dropped", p, dropped, before))
	}
	return out
}

func amount(variants []Variant) string {
	if len(variants) == 0 {
		return "—"
	}
	var low, high int64
	for i, v := range variants {
		n, _ := price(v.Price)
		if i == 0 || n < low {
			low = n
		}
		if i == 0 || n > high {
			high = n
		}
	}
	format := func(n int64) string {
		value := fmt.Sprintf("%d.%02d", n/100, n%100)
		if settings.Currency == "USD" {
			return "$" + value
		}
		return value + " " + settings.Currency
	}
	value := format(low)
	if low != high {
		value += "–" + format(high)
	}
	return value
}

func productEvent(kind string, p Product, variants, before []Variant) monitord.Event {
	link := strings.TrimRight(settings.Store, "/") + "/products/" + url.PathEscape(p.Handle)
	var labels []string
	for _, v := range variants {
		label := v.Title
		if label == "Default Title" {
			label = "One size"
		}
		labels = append(labels, label)
	}
	label := settings.Label
	if label == "" {
		u, _ := url.Parse(settings.Store)
		label = u.Hostname()
	}
	fields := []monitord.EventField{{Name: "Price", Value: amount(variants), Inline: true}}
	if len(before) > 0 {
		fields = append(fields, monitord.EventField{Name: "Previously", Value: amount(before), Inline: true})
	}
	if len(labels) > 0 {
		fields = append(fields, monitord.EventField{Name: "Sizes", Value: clip(strings.Join(labels, " · "), 800), Inline: true})
	}
	color := 0x71B89B
	if kind == "New drop" {
		color = 0xA99CE4
	}
	if kind == "Price dropped" {
		color = 0xD9B875
	}
	event := monitord.Event{ID: fmt.Sprintf("example:%d:%s", p.ID, kind), Title: p.Title, Description: "**" + kind + "** · " + label + "\n[View product ↗](" + link + ")", URL: link, Color: color, Fields: fields, Mentions: []string{}}
	if len(p.Images) > 0 {
		image := p.Images[0].URL
		if strings.HasPrefix(image, "//") {
			image = "https:" + image
		}
		if settings.ImageMode == "image" {
			event.Image = image
		}
		if settings.ImageMode == "thumbnail" {
			event.Thumbnail = image
		}
	}
	return event
}
