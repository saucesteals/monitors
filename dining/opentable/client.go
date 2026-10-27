package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
)

var proxySecret = monitord.RequiredSecret("proxies", "opentable")
var csrfPattern = regexp.MustCompile(`(?:__CSRF_TOKEN__|"__CSRF_TOKEN__"|"csrfToken")\s*[:=]\s*("(?:[^"\\]|\\.)*")`)

type client struct {
	pool       *httpx.ProxyClient
	jar        http.CookieJar
	csrf       string
	proxies    []string
	active     int
	retryAfter time.Time
}

// A one-entry pool pins bootstrap and availability to the same exit.
type selectedProxy string

func (s selectedProxy) Get(ref monitord.SecretRef) (string, bool) {
	return string(s), ref == proxySecret
}
func (s selectedProxy) Require(ref monitord.SecretRef) (string, error) {
	if ref != proxySecret {
		return "", errors.New("unknown proxy secret")
	}

	return string(s), nil
}

func newClient(secrets monitord.SecretSet) (*client, error) {
	raw, err := secrets.Require(proxySecret)
	if err != nil {
		return nil, errors.New("configure proxies/opentable")
	}
	c := &client{}
	if json.Unmarshal([]byte(raw), &c.proxies) != nil || len(c.proxies) == 0 {
		return nil, errors.New("proxy secret must be a nonempty JSON array")
	}
	for i, address := range c.proxies {
		u, err := url.Parse(address)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("invalid proxy entry %d", i)
		}
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(c.proxies))))
	if err != nil {
		return nil, err
	}
	c.active = int(index.Int64())

	return c, c.reset()
}

func (c *client) reset() error {
	if c.pool != nil {
		c.pool.CloseIdleConnections()
	}
	c.csrf = ""
	c.jar, _ = cookiejar.New(nil)
	raw, _ := json.Marshal([]string{c.proxies[c.active]})
	var err error
	c.pool, err = httpx.NewProxyClient(selectedProxy(raw), proxySecret)

	return err
}

type requestError struct {
	message   string
	transport bool
	status    int
}

func (e *requestError) Error() string { return e.message }

func (c *client) availability(ctx context.Context, date string, party, minutes int) ([]byte, error) {
	if time.Now().Before(c.retryAfter) {
		return nil, errors.New("server-requested cooldown active")
	}
	payload, err := availabilityPayload(date, party, minutes)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		body, err := c.fetch(attemptCtx, payload)
		cancel()
		if err == nil {
			return body, nil
		}
		var failure *requestError
		if !errors.As(err, &failure) || time.Now().Before(c.retryAfter) || ctx.Err() != nil {
			return nil, err
		}
		if !failure.transport && failure.status != 401 && failure.status != 403 && failure.status < 500 {
			return nil, err
		}
		// Refresh a broken connection on its existing exit once before rotating.
		if !failure.transport || attempt > 0 {
			c.active = (c.active + 1) % len(c.proxies)
		}
		slog.InfoContext(ctx, "refreshing anonymous session", "attempt", attempt+1, "reason", failure.message)
		if resetErr := c.reset(); resetErr != nil {
			return nil, resetErr
		}
		if attempt == 2 {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	panic("unreachable")
}

func (c *client) fetch(ctx context.Context, payload []byte) ([]byte, error) {
	if c.csrf == "" {
		address := settings.BootstrapURL
		if address == "" {
			address = settings.ProfileURL
		}
		body, err := c.read(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, err
		}
		match := csrfPattern.FindSubmatch(body)
		if len(match) != 2 || json.Unmarshal(match[1], &c.csrf) != nil || c.csrf == "" {
			return nil, &requestError{message: "bootstrap profile did not supply CSRF; check BootstrapURL", status: 403}
		}
	}

	return c.read(ctx, http.MethodPost, "https://www.opentable.com/dapi/fe/gql?optype=query&opname=RestaurantsAvailability", payload)
}

func (c *client) read(ctx context.Context, method, address string, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{
		"Accept-Language": {"en-US,en;q=0.9"}, "Cache-Control": {"no-cache"},
		"Accept":         {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"Sec-Fetch-Dest": {"document"}, "Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Site": {"none"},
	}
	if method == http.MethodPost {
		for name, value := range map[string]string{
			"Accept": "application/json", "Content-Type": "application/json",
			"Origin": "https://www.opentable.com", "Referer": settings.ProfileURL,
			"Ot-Page-Type": "restprofilepage", "Ot-Page-Group": "rest-profile",
			"X-CSRF-Token": c.csrf, "X-Query-Timeout": "5500",
			"Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-origin",
		} {
			req.Header.Set(name, value)
		}
	} else {
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	for _, cookie := range c.jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
	response, err := c.pool.Do(req)
	if err != nil {
		return nil, &requestError{message: "proxy transport failed", transport: true}
	}
	defer func() { _ = response.Body.Close() }()
	// The SDK owns redirects, but does not expose a native jar. Require
	// canonical endpoints rather than accepting incomplete redirect cookies.
	if response.Request.URL.String() != address {
		return nil, errors.New("endpoint redirected; configure its canonical URL")
	}
	c.jar.SetCookies(req.URL, response.Cookies())
	if response.StatusCode == http.StatusTooManyRequests {
		c.retryAfter = time.Now().Add(time.Minute)
		if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
			c.retryAfter = time.Now().Add(time.Duration(seconds) * time.Second)
		} else if at, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil && at.After(time.Now()) {
			c.retryAfter = at
		}
	}
	if response.StatusCode == http.StatusConflict {
		return nil, errors.New("HTTP 409: revalidate the persisted availability query")
	}
	if response.StatusCode != http.StatusOK {
		return nil, &requestError{message: fmt.Sprintf("opentable HTTP %d", response.StatusCode), status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil {
		return nil, &requestError{message: "response body transport failed", transport: true}
	}
	if len(body) > 8<<20 {
		return nil, errors.New("response exceeds 8 MiB")
	}

	return body, nil
}
