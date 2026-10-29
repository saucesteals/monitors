package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
)

// ispPool keeps a bounded working set warm instead of cold-connecting through
// hundreds of proxies. A failed working set advances through the original pool.
// Credentials stay in memory and are never copied to state, files or logs.
type ispPool struct {
	values  []string
	next    int
	size    int
	current *httpx.ProxyClient
}

type selectedSecrets struct {
	monitord.SecretSet
	value string
}

func (s selectedSecrets) Get(ref monitord.SecretRef) (string, bool) {
	if ref.Group == proxies.Group && ref.Key == proxies.Key {
		return s.value, true
	}

	return s.SecretSet.Get(ref)
}

func (s selectedSecrets) Require(ref monitord.SecretRef) (string, error) {
	if ref.Group == proxies.Group && ref.Key == proxies.Key {
		return s.value, nil
	}

	return s.SecretSet.Require(ref)
}

func newISPPool(secrets monitord.SecretSet) (*ispPool, error) {
	raw, err := secrets.Require(proxies)
	if err != nil {
		return nil, err
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil || len(values) == 0 {
		return nil, errors.New("ISP pool must be a nonempty JSON array")
	}
	// Revisit each exit within the transport's 90-second idle timeout at
	// frequent cadences. Slow schedules need only one reusable exit.
	size := min(4, max(1, int(80*time.Second/(settings.Interval+requestTimeout))))
	p := &ispPool{values: values, size: min(size, len(values))}
	rand.Shuffle(len(values), func(i, j int) { values[i], values[j] = values[j], values[i] })
	if err := p.rotate(secrets); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *ispPool) rotate(secrets monitord.SecretSet) error {
	selected := make([]string, p.size)
	for i := range selected {
		selected[i] = p.values[(p.next+i)%len(p.values)]
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		return errors.New("encode selected ISP pool")
	}
	client, err := httpx.NewProxyClient(selectedSecrets{
		SecretSet: secrets,
		value:     string(encoded),
	}, proxies)
	if err != nil {
		return fmt.Errorf("initialize ISP working set: %w", err)
	}
	if p.current != nil {
		p.current.CloseIdleConnections()
	}
	p.current = client
	p.next = (p.next + len(selected)) % len(p.values)

	return nil
}
