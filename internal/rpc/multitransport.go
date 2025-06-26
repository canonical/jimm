// Copyright 2025 Canonical.

package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// multiBackendTransport is a custom HTTP transport that attempts to make requests
// to multiple backend URLs in succession until one succeeds. It will not
// continue to retry if a request fails partway through (e.g., if the body
// was partially read), as we don't store the original request body.
//
// Reusing a multiBackendTransport object will ensure that requests
// are made in a round-robin fashion across the provided URLs.
//
// If a new multiBackendTransport is created per request, it will always start
// from the first URL, so the caller should randomise the list of URLs first.
type multiBackendTransport struct {
	// baseTransport is the underlying transport used for actual HTTP requests
	baseTransport http.RoundTripper
	// urls contains the list of backend urls to try in order
	urls []*url.URL
	// currentURLIndex tracks which URL should be tried next (for round-robin)
	currentURLIndex int
}

// newMultiBackendTransport creates a new MultiBackendTransport with the given
// base transport and URLs.
func newMultiBackendTransport(baseTransport http.RoundTripper, urls []*url.URL) (*multiBackendTransport, error) {
	if len(urls) == 0 {
		return nil, errors.New("no backend URLs configured")
	}
	return &multiBackendTransport{
		baseTransport: baseTransport,
		urls:          urls,
	}, nil
}

// RoundTrip implements the http.RoundTripper interface and attempts to make
// requests to multiple backends in succession.
func (m *multiBackendTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error

	// Try each URL in succession
	for i := 0; i < len(m.urls); i++ {
		// Calculate the URL index to try (round-robin starting from currentURLIndex)
		urlIndex := (m.currentURLIndex + i) % len(m.urls)
		targetURL := m.urls[urlIndex]

		// Clone the request to avoid modifying the original
		clonedReq := req.Clone(req.Context())
		clonedReq.URL.Scheme = targetURL.Scheme
		clonedReq.URL.Host = targetURL.Host
		newPath, err := url.JoinPath("/", targetURL.Path, clonedReq.URL.Path)
		if err != nil {
			zapctx.Debug(context.Background(), "failed to join URL path", zap.Error(err))
			lastErr = err
			continue
		}
		clonedReq.URL.Path = newPath
		clonedReq.Host = targetURL.Host

		// Attempt the request
		resp, err := m.baseTransport.RoundTrip(clonedReq)
		if err == nil {
			// Success - update the current URL index for next time (round-robin)
			m.currentURLIndex = (urlIndex + 1) % len(m.urls)
			return resp, nil
		}
		if err == io.EOF {
			// If we hit EOF, it likely means that we failed partway through
			// the request and without the original body of the request
			// stored, we cannot safely retry.
			return nil, errors.New("backend failure during request")
		}

		// Log the failure and try the next URL
		zapctx.Debug(context.Background(), "request to backend failed, trying next",
			zap.String("backend", targetURL.String()),
			zap.Error(err),
			zap.Int("attempt", i+1),
			zap.Int("total_backends", len(m.urls)))

		lastErr = err
	}

	// All backends failed
	return nil, fmt.Errorf("all backends failed, last error: %w", lastErr)
}
