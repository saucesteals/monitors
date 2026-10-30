package main

import "net/http"

// Keep only the numeric-city routing adapter. All location resolution is Shop's.
type burlingtonTransport struct {
	base interface {
		Do(*http.Request) (*http.Response, error)
		CloseIdleConnections()
	}
}

func (t *burlingtonTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "www.facebook.com" && req.URL.Path == "/marketplace/burlingtonct/search/" {
		req = req.Clone(req.Context())
		req.URL.Path = "/marketplace/109423279083274/search/"
	}
	return t.base.Do(req)
}
func (t *burlingtonTransport) CloseIdleConnections() { t.base.CloseIdleConnections() }
