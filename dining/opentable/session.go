package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/fhttp/cookiejar"
	"github.com/saucesteals/mimic"
	"github.com/saucesteals/monitord"
)

var proxySecret = monitord.RequiredSecret("proxies", "opentable")

// The SDK httpx helper pins Chrome 131 and has no fingerprint option.
// OpenTable requires a newer profile, so use the same Mimic dependency with
// an explicit version and a native cookie jar on each proxy-bound client.
const browserVersion = "147.0.0.0"

type browserSession struct {
	client *http.Client
	proxy  *url.URL
	csrf   string
}

func (m *monitor) startSessions(secrets monitord.SecretSet) (err error) {
	defer func() {
		if err != nil {
			_ = m.Stop(context.Background())
		}
	}()
	values := []string{""}
	if settings.UseProxy {
		if secrets == nil {
			return errors.New("proxies/opentable is required")
		}
		raw, err := secrets.Require(proxySecret)
		if err != nil {
			return errors.New("proxies/opentable is unavailable")
		}
		if json.Unmarshal([]byte(raw), &values) != nil || len(values) == 0 {
			return errors.New("proxies/opentable must be a nonempty JSON array of proxy URLs")
		}
	}
	for i, value := range values {
		session := &browserSession{}
		if settings.UseProxy {
			// Validate without including secret URLs in errors.
			parsed, err := url.Parse(value)
			if err != nil || parsed.Hostname() == "" || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5") {
				return fmt.Errorf("invalid proxy at index %d", i)
			}
			session.proxy = parsed
		}
		if err := session.reset(); err != nil {
			return fmt.Errorf("initialize session at index %d: %w", i, err)
		}
		m.sessions = append(m.sessions, session)
	}
	// Spread independent worker starts over the pool without publishing order.
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(m.sessions))))
	if err != nil {
		return errors.New("select initial proxy")
	}
	m.active = int(index.Int64())

	return nil
}

func (s *browserSession) reset() error {
	if s.client != nil {
		s.client.CloseIdleConnections()
	}
	s.csrf = ""
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("reset cookies: %w", err)
	}
	var proxy func(*http.Request) (*url.URL, error)
	if s.proxy != nil {
		proxy = http.ProxyURL(s.proxy)
	}
	transport, err := mimic.NewTransport(mimic.TransportOptions{
		Version:   browserVersion,
		Brand:     mimic.BrandChrome,
		Platform:  mimic.PlatformMac,
		Transport: &http.Transport{Proxy: proxy},
	})
	if err != nil {
		return errors.New("initialize browser transport")
	}
	s.client = &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   20 * time.Second,
	}

	return nil
}

type accessError struct{ reason string }

func (e *accessError) Error() string { return e.reason }

func (m *monitor) fetch(ctx context.Context, date string) ([]string, error) {
	var last error
	// At most three complete session attempts, including bootstrap failures.
	// One-proxy/direct mode retries once with fresh cookies and CSRF.
	attempts := min(3, len(m.sessions)+1)
	for attempt := range attempts {
		session := m.sessions[m.active]
		slots, err := m.fetchSession(ctx, session, date)
		if err == nil {
			return slots, nil
		}
		last = err
		var access *accessError
		if !errors.As(err, &access) || ctx.Err() != nil {
			return nil, err
		}
		if err := session.reset(); err != nil {
			return nil, err
		}
		m.active = (m.active + 1) % len(m.sessions)
		if attempt+1 < attempts {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}

	return nil, fmt.Errorf("session attempts exhausted: %w", last)
}
