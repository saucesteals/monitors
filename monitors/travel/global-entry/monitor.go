package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
	_ "time/tzdata"

	"github.com/saucesteals/monitord"
)

type State struct{}
type Snapshot struct {
	Earliest string `json:"earliest"`
	Sequence uint64 `json:"sequence"`
}

type Slot struct {
	Active     bool   `json:"active"`
	LocationID int    `json:"locationId"`
	Start      string `json:"startTimestamp"`
	End        string `json:"endTimestamp"`
	Duration   int    `json:"duration"`
}

type monitor struct {
	client *http.Client
	zone   *time.Location
	before time.Time
}

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "global-entry", Description: "An earlier Global Entry interview opens"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(20*time.Second))
}

func (m *monitor) Start(context.Context, monitord.Environment) error {
	if settings.LocationID <= 0 || settings.Label == "" || settings.Interval < 30*time.Second {
		return fmt.Errorf("configure a location ID and label")
	}
	var err error
	m.zone, err = time.LoadLocation(settings.Timezone)
	if err != nil {
		return err
	}
	if settings.Before != "" {
		m.before, err = time.ParseInLocation("2006-01-02", settings.Before, m.zone)
		if err != nil {
			return err
		}
	}
	m.client = &http.Client{Timeout: 15 * time.Second, Transport: http.DefaultTransport.(*http.Transport).Clone()}
	return nil
}

func (m *monitor) Stop(context.Context) error {
	if m.client != nil {
		m.client.CloseIdleConnections()
	}
	return nil
}
