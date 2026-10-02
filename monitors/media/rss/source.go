package main

import (
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"github.com/saucesteals/monitord"
	"golang.org/x/net/html"
	"net/url"
	"sort"
	"strings"
)

type Entry struct {
	ID, Title, URL, Summary, Image, Published string
	Categories                                []string
}

type RSSItem struct {
	ID         string   `xml:"guid"`
	Title      string   `xml:"title"`
	URL        string   `xml:"link"`
	Summary    string   `xml:"description"`
	Content    string   `xml:"encoded"`
	Published  string   `xml:"pubDate"`
	Categories []string `xml:"category"`
	Enclosure  struct {
		URL  string `xml:"url,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
	Thumbnail struct {
		URL string `xml:"url,attr"`
	} `xml:"thumbnail"`
}

type AtomItem struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Content   string `xml:"content"`
	Published string `xml:"published"`
	Links     []struct {
		URL string `xml:"href,attr"`
		Rel string `xml:"rel,attr"`
	} `xml:"link"`
	Categories []struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
}

func validate() error {
	u, e := url.Parse(settings.URL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("configure an HTTPS feed URL")
	}
	return nil
}

func absolute(raw string) string {
	base, e := url.Parse(settings.URL)
	if e != nil {
		return ""
	}
	u, e := url.Parse(raw)
	if e != nil || raw == "" {
		return ""
	}
	u = base.ResolveReference(u)
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.String()
}

func plain(markup string) (string, string) {
	z := html.NewTokenizer(strings.NewReader(markup))
	var words []string
	image := ""
	skip := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(strings.Join(words, " ")), " "), image
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data == "script" || t.Data == "style" {
				skip = true
			}
			if t.Data == "img" && image == "" {
				for _, a := range t.Attr {
					if a.Key == "src" {
						image = absolute(a.Val)
					}
				}
			}
		case html.EndTagToken:
			t := z.Token()
			if t.Data == "script" || t.Data == "style" {
				skip = false
			}
		case html.TextToken:
			if !skip {
				words = append(words, string(z.Text()))
			}
		}
	}
}

func parseFeed(b []byte) (string, []Entry, error) {
	var root struct{ XMLName xml.Name }
	if e := xml.Unmarshal(b, &root); e != nil {
		return "", nil, e
	}
	var title string
	var out []Entry
	switch root.XMLName.Local {
	case "rss":
		var feed struct {
			Channel struct {
				Title string    `xml:"title"`
				Items []RSSItem `xml:"item"`
			} `xml:"channel"`
		}
		if e := xml.Unmarshal(b, &feed); e != nil {
			return "", nil, e
		}
		title = feed.Channel.Title
		for _, i := range feed.Channel.Items {
			text, _ := plain(i.Summary)
			_, image := plain(i.Content)
			if image == "" {
				image = absolute(i.Thumbnail.URL)
			}
			if image == "" && strings.HasPrefix(i.Enclosure.Type, "image/") {
				image = absolute(i.Enclosure.URL)
			}
			id := i.ID
			if id == "" {
				id = i.URL
			}
			out = append(out, Entry{ID: id, Title: i.Title, URL: absolute(i.URL), Summary: text, Image: image, Published: i.Published, Categories: i.Categories})
		}
	case "feed":
		var feed struct {
			Title string     `xml:"title"`
			Items []AtomItem `xml:"entry"`
		}
		if e := xml.Unmarshal(b, &feed); e != nil {
			return "", nil, e
		}
		title = feed.Title
		for _, i := range feed.Items {
			link := ""
			for _, l := range i.Links {
				if l.Rel == "alternate" || l.Rel == "" {
					link = absolute(l.URL)
					break
				}
			}
			text, image := plain(i.Summary)
			if text == "" {
				text, image = plain(i.Content)
			}
			var categories []string
			for _, c := range i.Categories {
				categories = append(categories, c.Term)
			}
			out = append(out, Entry{ID: i.ID, Title: i.Title, URL: link, Summary: text, Image: image, Published: i.Published, Categories: categories})
		}
	default:
		return "", nil, fmt.Errorf("expected RSS 2.0 or Atom")
	}
	if title == "" {
		return "", nil, fmt.Errorf("feed title missing")
	}
	for _, i := range out {
		if i.ID == "" || i.Title == "" || i.URL == "" {
			return "", nil, fmt.Errorf("entry lacks stable identity, title, or link")
		}
	}
	return title, out, nil
}

func (m *monitor) fetch(ctx context.Context) ([]Observation, error) {
	b, e := m.get(ctx, settings.URL)
	if e != nil {
		return nil, e
	}
	title, entries, e := parseFeed(b)
	if e != nil {
		return nil, e
	}
	// Feed order is not an identity. Stable IDs dedupe edits and reordering.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	var out []Observation
	for _, entry := range entries {
		category := settings.Category == ""
		for _, c := range entry.Categories {
			if strings.EqualFold(c, settings.Category) {
				category = true
			}
		}
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(entry.ID)))
		out = append(out, Observation{ID: id, Event: entryEvent(title, entry), Match: category && matches(entry.Title, settings.TitleContains)})
	}
	return out, nil
}

func entryEvent(feed string, e Entry) monitord.Event {
	description := "**" + feed + "**"
	if e.Summary != "" {
		description += "\n\n" + clip(e.Summary, 420)
	}
	description += "\n\n[Read article ↗](" + e.URL + ")"
	return monitord.Event{ID: "feed:" + fmt.Sprintf("%x", sha256.Sum256([]byte(e.ID))), Title: clip(e.Title, 220), Description: description, URL: e.URL, Image: e.Image, Color: 0xD9A877, Mentions: []string{}}
}
