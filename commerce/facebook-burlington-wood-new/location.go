package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// The pinned SDK accepts only alphabetic city slugs. Resolve this monitor's
// dedicated alias to Burlington CT, and expand state abbreviations because
// Facebook's place typeahead returns empty results for many "Town, CT" queries.
type burlingtonTransport struct {
	base interface {
		Do(*http.Request) (*http.Response, error)
		CloseIdleConnections()
	}
}

func (t *burlingtonTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "www.facebook.com" {
		return t.base.Do(req)
	}
	if req.URL.Path == "/marketplace/burlingtonct/search/" {
		req = req.Clone(req.Context())
		req.URL.Path = "/marketplace/109423279083274/search/"
	}
	if req.URL.Path == "/api/graphql/" && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("read location request: %w", err)
		}
		raw, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			return nil, fmt.Errorf("read location form: %w", err)
		}
		form, err := url.ParseQuery(string(raw))
		if err != nil {
			return nil, fmt.Errorf("parse location form: %w", err)
		}
		if form.Get("fb_api_req_friendly_name") == "MarketplaceSearchAddressDataSourceQuery" {
			variables := strings.NewReplacer(
				`, CT"`, `, Connecticut"`,
				`, MA"`, `, Massachusetts"`,
				`, NY"`, `, New York"`,
				`, RI"`, `, Rhode Island"`,
			).Replace(form.Get("variables"))
			form.Set("variables", variables)
			encoded := form.Encode()
			req = req.Clone(req.Context())
			req.Body = io.NopCloser(strings.NewReader(encoded))
			req.ContentLength = int64(len(encoded))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(encoded)), nil
			}
		}
	}

	return t.base.Do(req)
}

func (t *burlingtonTransport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
}
