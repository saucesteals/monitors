package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/saucesteals/monitord"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Video struct {
	ID        string `xml:"videoId"`
	ChannelID string `xml:"channelId"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Media struct {
		Description string `xml:"description"`
		Thumbnail   struct {
			URL string `xml:"url,attr"`
		} `xml:"thumbnail"`
	} `xml:"group"`
}

type Feed struct {
	XMLName   xml.Name
	ChannelID string  `xml:"channelId"`
	Videos    []Video `xml:"entry"`
}

func validate() error {
	if !regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`).MatchString(settings.ChannelID) {
		return fmt.Errorf("configure a YouTube channel ID beginning UC")
	}
	return nil
}

func (m *monitor) fetch(ctx context.Context) ([]Observation, error) {
	b, e := m.get(ctx, "https://www.youtube.com/feeds/videos.xml?channel_id="+settings.ChannelID)
	if e != nil {
		return nil, e
	}
	var feed Feed
	if e = xml.Unmarshal(b, &feed); e != nil || feed.XMLName.Local != "feed" || normalizeChannel(feed.ChannelID) != settings.ChannelID {
		return nil, fmt.Errorf("invalid YouTube feed or channel mismatch")
	}
	sort.Slice(feed.Videos, func(i, j int) bool { return feed.Videos[i].Published < feed.Videos[j].Published })
	var out []Observation
	for _, v := range feed.Videos {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`).MatchString(v.ID) || v.Title == "" || normalizeChannel(v.ChannelID) != settings.ChannelID {
			return nil, fmt.Errorf("incomplete video")
		}
		out = append(out, Observation{ID: v.ID, Match: matches(v.Title, settings.TitleContains), Event: videoEvent(v)})
	}
	return out, nil
}

func videoEvent(v Video) monitord.Event {
	link := "https://www.youtube.com/watch?v=" + v.ID
	description := "**" + v.Author.Name + "** · New upload"
	summary := strings.SplitN(strings.TrimSpace(v.Media.Description), "\n\n", 2)[0]
	if summary != "" {
		description += "\n\n" + clip(summary, 220)
	}
	description += "\n\n[Watch video ↗](" + link + ")"
	date := v.Published
	if t, e := time.Parse(time.RFC3339, date); e == nil {
		date = t.Format("Jan 2, 2006")
	}
	return monitord.Event{ID: "youtube:" + v.ID, Title: v.Title, Description: description, URL: link, Image: v.Media.Thumbnail.URL, Color: 0xE27676, Fields: []monitord.EventField{{Name: "Published", Value: date}}, Mentions: []string{}}
}

// YouTube omits the UC prefix on the feed-level ID, but includes it on entries.
func normalizeChannel(value string) string {
	if len(value) == 22 {
		return "UC" + value
	}
	return value
}
