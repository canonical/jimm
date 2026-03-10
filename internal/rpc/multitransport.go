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

// readTrackingNoOpCloserBody wraps an io.ReadCloser to track whether it has been read from
// and provides a no-op Close method to allow the original request body to be closed separately.
// This is similar to the stdlib's net/http/transport.go readTrackingBody but adds a no-op closer.
type readTrackingNoOpCloserBody struct {
	io.ReadCloser
	didRead bool
}

// Read tracks that the body has been read from and delegates to the underlying ReadCloser.
func (r *readTrackingNoOpCloserBody) Read(data []byte) (int, error) {
	r.didRead = true
	return r.ReadCloser.Read(data)
}

// Close is a no-op to allow the original request body to be closed separately.
func (r *readTrackingNoOpCloserBody) Close() error {
	return nil
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

	// Track the original request body to determine if we can safely retry on failure
	trackedRequest := req
	var trackedBody *readTrackingNoOpCloserBody
	if req.Body != nil {
		// We do not close the original request body, this is done
		// by the HTTP server that accepts the request, see the godoc
		// for the http.Request Body field.
		trackedBody = &readTrackingNoOpCloserBody{ReadCloser: req.Body}
		trackedRequest = req.Clone(req.Context())
		trackedRequest.Body = trackedBody
	}

	// Try each URL in succession
	for i := 0; i < len(m.urls); i++ {
		// Calculate the URL index to try (round-robin starting from currentURLIndex)
		urlIndex := (m.currentURLIndex + i) % len(m.urls)
		targetURL := m.urls[urlIndex]

		// Clone the request to avoid modifying the original
		clonedReq := trackedRequest.Clone(trackedRequest.Context())
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
		if trackedBody != nil && trackedBody.didRead {
			// If we've read from the body we've lost some data and cannot retry, so return the error.
			return nil, fmt.Errorf("non-retryable backend failure during request: %w", err)
		}

		// Log the failure and try the next URL
		zapctx.Error(context.Background(), "request to backend failed, trying next",
			zap.String("backend", targetURL.String()),
			zap.Error(err),
			zap.Int("attempt", i+1),
			zap.Int("total_backends", len(m.urls)))

		lastErr = err
	}

	// All backends failed
	return nil, fmt.Errorf("all backends failed, last error: %w", lastErr)
}
