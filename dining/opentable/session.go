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
	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/httpx"
)

var proxySecret = monitord.RequiredSecret("proxies", "opentable")

type browserClient interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

// Each SDK pool contains exactly one proxy: profile and API calls cannot
// rotate independently. Cookies and CSRF are never shared between exits.
type browserSession struct {
	client    browserClient
	newClient func() (browserClient, error)
	jar       http.CookieJar
	csrf      string
}

type singleProxySecret string

func (s singleProxySecret) Get(ref monitord.SecretRef) (string, bool) {
	if ref.Group != proxySecret.Group || ref.Key != proxySecret.Key {
		return "", false
	}

	return string(s), true
}

func (s singleProxySecret) Require(ref monitord.SecretRef) (string, error) {
	value, ok := s.Get(ref)
	if !ok {
		return "", errors.New("unknown proxy secret")
	}

	return value, nil
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
		var create func() (browserClient, error)
		if settings.UseProxy {
			// Validate without including secret URLs in errors.
			parsed, err := url.Parse(value)
			if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5") {
				return fmt.Errorf("invalid proxy at index %d", i)
			}
			raw, err := json.Marshal([]string{value})
			if err != nil {
				return errors.New("encode proxy configuration")
			}
			create = func() (browserClient, error) {
				return httpx.NewProxyClient(singleProxySecret(raw), proxySecret)
			}
		} else {
			create = func() (browserClient, error) {
				return httpx.NewClient()
			}
		}
		session := &browserSession{newClient: create}
		if err := session.reset(); err != nil {
			return fmt.Errorf("initialize session at index %d", i)
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
	client, err := s.newClient()
	if err != nil {
		return errors.New("initialize browser transport")
	}
	s.client = client
	s.jar = jar

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
