package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/saucesteals/monitord"
)

var githubToken = monitord.OptionalSecret("github", "token")

type Release struct {
	ID         int64  `json:"id"`
	Tag        string `json:"tag_name"`
	Name       string `json:"name"`
	URL        string `json:"html_url"`
	Body       string `json:"body"`
	Published  string `json:"published_at"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Author     struct {
		Avatar string `json:"avatar_url"`
	} `json:"author"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func validate() error {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(settings.Repository) {
		return fmt.Errorf("repository must be owner/name")
	}
	return nil
}

func (m *monitor) fetch(ctx context.Context) ([]Observation, error) {
	b, err := m.get(ctx, "https://api.github.com/repos/"+settings.Repository+"/releases?per_page=100")
	if err != nil {
		return nil, err
	}
	var releases []Release
	if err = json.Unmarshal(b, &releases); err != nil || releases == nil {
		return nil, fmt.Errorf("invalid releases array")
	}
	sort.Slice(releases, func(i, j int) bool { return releases[i].Published < releases[j].Published })
	out := make([]Observation, 0, len(releases))
	for _, r := range releases {
		if r.Draft {
			continue
		}
		if r.ID == 0 || r.Tag == "" || r.URL == "" {
			return nil, fmt.Errorf("incomplete release")
		}
		// Promotion from prerelease to stable is a separate useful occurrence.
		id := strconv.FormatInt(r.ID, 10) + ":" + strconv.FormatBool(r.Prerelease)
		match := !r.Draft && (settings.IncludePrereleases || !r.Prerelease) && strings.HasPrefix(r.Tag, settings.TagPrefix)
		out = append(out, Observation{ID: id, Event: releaseEvent(r), Match: match})
	}
	return out, nil
}

func releaseEvent(r Release) monitord.Event {
	channel := "Stable release"
	if r.Prerelease {
		channel = "Prerelease"
	}
	title := r.Name
	if title == "" {
		title = r.Tag
	}
	// Release notes are Markdown already. Take a short first paragraph, not a wall of text.
	var paragraphs []string
	for _, p := range strings.Split(strings.ReplaceAll(r.Body, "\r\n", "\n"), "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		paragraphs = append(paragraphs, p)
		break
	}
	description := "**" + settings.Repository + "** · " + channel
	if len(paragraphs) > 0 {
		description += "\n\n" + clip(paragraphs[0], 450)
	}
	description += "\n\n[Release notes & downloads ↗](" + r.URL + ")"
	return monitord.Event{ID: fmt.Sprintf("release:%d", r.ID), Title: title, URL: r.URL, Description: description, Thumbnail: r.Author.Avatar, Color: 0xA99CE4, Fields: []monitord.EventField{{Name: "Version", Value: "`" + r.Tag + "`", Inline: true}, {Name: "Downloads", Value: strconv.Itoa(len(r.Assets)) + " assets", Inline: true}}, Mentions: []string{}}
}
