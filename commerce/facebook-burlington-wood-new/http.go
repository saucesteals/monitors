package main

import "net/http"

// Adapt the lifecycle-owned proxy client to net/http without changing requests.
type facebookTransport struct {
	base interface {
		Do(*http.Request) (*http.Response, error)
		CloseIdleConnections()
	}
}

func (t *facebookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.Do(req)
}
func (t *facebookTransport) CloseIdleConnections() { t.base.CloseIdleConnections() }
