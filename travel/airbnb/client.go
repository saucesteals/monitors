package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
)

var proxySecret = monitord.RequiredSecret("proxies", "airbnb")

type transport interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("Airbnb HTTP %d", e.code)
}

type source struct {
	config     config
	client     transport
	operations map[string]string
	refreshed  time.Time
}

func newSource(c config, secrets monitord.SecretSet) (*source, error) {
	var client transport
	var err error
	if c.UseProxy {
		client, err = httpx.NewProxyClient(secrets, proxySecret)
	} else {
		client, err = httpx.NewClient()
	}
	if err != nil {
		return nil, fmt.Errorf("create Airbnb client: %w", err)
	}

	return &source{
		config: c,
		client: client,
	}, nil
}

func (s *source) invalidate() {
	s.operations = nil
}

func (s *source) get(ctx context.Context, address, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if key != "" {
		req.Header.Set("X-Airbnb-API-Key", key)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Airbnb request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Request != nil && resp.Request.URL.Host != req.URL.Host {
		return nil, fmt.Errorf("Airbnb redirected to a different host")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{code: resp.StatusCode}
	}
	const limit = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read Airbnb response: %w", err)
	}
	if len(body) > limit {
		return nil, fmt.Errorf("Airbnb response exceeds 8 MiB")
	}

	return body, nil
}

type persistedQuery struct {
	Version int    `json:"version"`
	Hash    string `json:"sha256Hash"`
}

// query leaves response decoding to the operation-specific typed boundary.
func (s *source) query(ctx context.Context, key, operation string, variables json.RawMessage) ([]byte, error) {
	hash := s.operations[operation]
	if hash == "" {
		return nil, fmt.Errorf("query artifact missing for %s", operation)
	}
	extensions, err := json.Marshal(struct {
		Query persistedQuery `json:"persistedQuery"`
	}{Query: persistedQuery{Version: 1, Hash: hash}})
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"operationName": {operation}, "locale": {"en"}, "currency": {s.config.Currency},
		"variables": {string(variables)}, "extensions": {string(extensions)},
	}
	body, err := s.get(ctx, "https://www.airbnb.com/api/v3/"+operation+"/"+hash+"?"+q.Encode(), key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}

	return body, nil
}

var (
	bootstrapPattern = regexp.MustCompile(`(?s)<script[^>]*id="data-deferred-state-0"[^>]*>(.*?)</script>`)
	keyPattern       = regexp.MustCompile(`"api_config":\s*\{\s*"key":"([^"]+)"`)
	assetPattern     = regexp.MustCompile(`https://a0\.muscache\.com/[^"<>\s]+\.js`)
)

func bootstrapVariables(page []byte, operation string) (json.RawMessage, error) {
	match := bootstrapPattern.FindSubmatch(page)
	if len(match) != 2 {
		return nil, fmt.Errorf("Airbnb bootstrap absent; possible challenge or schema change")
	}
	var state struct {
		Entries [][]json.RawMessage `json:"niobeClientData"`
	}
	if err := json.Unmarshal(match[1], &state); err != nil {
		return nil, fmt.Errorf("decode Airbnb bootstrap: %w", err)
	}
	prefix := operation + ":"
	for _, pair := range state.Entries {
		var name string
		if len(pair) < 2 || json.Unmarshal(pair[0], &name) != nil || !strings.HasPrefix(name, prefix) {
			continue
		}
		variables := json.RawMessage(strings.TrimPrefix(name, prefix))
		if !json.Valid(variables) {
			return nil, fmt.Errorf("invalid %s bootstrap variables", operation)
		}

		return variables, nil
	}

	return nil, fmt.Errorf("%s absent from Airbnb bootstrap", operation)
}

// discoverOperations reads declarative query identifiers as text. Downloaded
// JavaScript is never executed. Per discovery, at most 48 same-CDN assets and
// 32 MiB of asset bodies are examined; failures are not empty availability.
func (s *source) discoverOperations(ctx context.Context, page []byte, names ...string) error {
	if s.operations == nil || time.Since(s.refreshed) > 24*time.Hour {
		s.operations = make(map[string]string)
		s.refreshed = time.Now()
	}
	missing := make(map[string]*regexp.Regexp)
	for _, name := range names {
		if s.operations[name] == "" {
			missing[name] = regexp.MustCompile(`name:["']` + regexp.QuoteMeta(name) + `["'],type:["']query["'],operationId:["']([a-f0-9]{64})["']`)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	assets := assetPattern.FindAllString(string(page), -1)
	// Route bundles normally carry PDP queries. Search artifact names are
	// content hashed, so no build-specific filename is embedded here.
	sort.SliceStable(assets, func(i, j int) bool {
		return assetRank(assets[i]) < assetRank(assets[j])
	})
	seen := make(map[string]bool)
	bytesRead := 0
	for _, address := range assets {
		if seen[address] {
			continue
		}
		seen[address] = true
		if len(seen) > 48 || bytesRead >= 32<<20 {
			break
		}
		body, err := s.get(ctx, address, "")
		if err != nil {
			var status *statusError
			if errors.As(err, &status) && status.code == http.StatusNotFound {
				continue // Unrelated/stale asset references are not query data.
			}

			return fmt.Errorf("read Airbnb query artifact: %w", err)
		}
		bytesRead += len(body)
		if bytesRead > 32<<20 {
			return fmt.Errorf("Airbnb query artifact byte bound exceeded")
		}
		for name, pattern := range missing {
			match := pattern.FindSubmatch(body)
			if len(match) == 2 {
				s.operations[name] = string(match[1])
				delete(missing, name)
			}
		}
		if len(missing) == 0 {
			return nil
		}
	}
	var absent []string
	for name := range missing {
		absent = append(absent, name)
	}
	sort.Strings(absent)

	return fmt.Errorf("Airbnb query artifacts not found within discovery bounds: %s", strings.Join(absent, ", "))
}

func assetRank(address string) int {
	if strings.Contains(address, "/PdpPlatformRoute.") {
		return 0
	}
	if strings.Contains(address, "/common/") {
		return 2
	}

	return 1
}
