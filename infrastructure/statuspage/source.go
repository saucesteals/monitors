package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/saucesteals/monitord"
	"net/url"
	"sort"
	"strings"
)

type Update struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Body    string `json:"body"`
	Created string `json:"created_at"`
}

type Incident struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Impact     string `json:"impact"`
	URL        string `json:"shortlink"`
	Components []struct {
		Name string `json:"name"`
	} `json:"components"`
	Updates []Update `json:"incident_updates"`
}

func impact(s string) int {
	switch s {
	case "none":
		return 0
	case "minor":
		return 1
	case "major":
		return 2
	case "critical":
		return 3
	}
	return -1
}

func validate() error {
	u, e := url.Parse(settings.URL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return fmt.Errorf("configure an HTTPS status page")
	}
	if impact(settings.MinimumImpact) < 0 {
		return fmt.Errorf("unknown minimum impact")
	}
	return nil
}

func (m *monitor) fetch(ctx context.Context) ([]Observation, error) {
	b, e := m.get(ctx, strings.TrimRight(settings.URL, "/")+"/api/v2/incidents.json")
	if e != nil {
		return nil, e
	}
	var payload struct {
		Page struct {
			Name string `json:"name"`
		} `json:"page"`
		Incidents *[]Incident `json:"incidents"`
	}
	if e = json.Unmarshal(b, &payload); e != nil || payload.Page.Name == "" || payload.Incidents == nil {
		return nil, fmt.Errorf("invalid status page response")
	}
	var out []Observation
	for _, incident := range *payload.Incidents {
		if incident.ID == "" || incident.Name == "" || len(incident.Updates) == 0 {
			return nil, fmt.Errorf("incomplete incident")
		}
		matched := len(settings.Components) == 0
		for _, c := range incident.Components {
			for _, want := range settings.Components {
				if strings.EqualFold(c.Name, want) {
					matched = true
				}
			}
		}
		sort.Slice(incident.Updates, func(i, j int) bool { return incident.Updates[i].Created < incident.Updates[j].Created })
		for _, u := range incident.Updates {
			if u.ID == "" || u.Status == "" {
				return nil, fmt.Errorf("incomplete incident update")
			}
			out = append(out, Observation{ID: u.ID, Event: incidentEvent(payload.Page.Name, incident, u), Match: matched && impact(incident.Impact) >= impact(settings.MinimumImpact)})
		}
	}
	return out, nil
}

func incidentEvent(page string, i Incident, u Update) monitord.Event {
	color := 0xD9B875
	if u.Status == "resolved" {
		color = 0x71B89B
	}
	if i.Impact == "critical" && u.Status != "resolved" {
		color = 0xD98282
	}
	status := strings.ReplaceAll(u.Status, "_", " ")
	if len(status) > 0 {
		status = strings.ToUpper(status[:1]) + status[1:]
	}
	link := i.URL
	if link == "" {
		link = strings.TrimRight(settings.URL, "/") + "/incidents/" + i.ID
	}
	var components []string
	for _, c := range i.Components {
		components = append(components, c.Name)
	}
	fields := []monitord.EventField{{Name: "Status", Value: status, Inline: true}, {Name: "Impact", Value: i.Impact, Inline: true}}
	if len(components) > 0 {
		fields = append(fields, monitord.EventField{Name: "Affected", Value: clip(strings.Join(components, " · "), 500)})
	}
	return monitord.Event{ID: "incident:" + u.ID, Title: i.Name, URL: link, Description: "**" + page + "**\n\n" + clip(u.Body, 650) + "\n\n[View incident ↗](" + link + ")", Color: color, Fields: fields, Mentions: []string{}}
}
