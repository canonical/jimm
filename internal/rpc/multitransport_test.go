// Copyright 2025 Canonical.

package rpc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/rpc"
)

// TestMultiBackendTransport tests the multiBackendTransport
// implementation by simulating various server responses and ensuring
// that the transport correctly handles them specifically for failover scenarios.
func TestMultiBackendTransport(t *testing.T) {
	c := qt.New(t)
	expectedBody := "this is the request body"

	// Success server
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
		if string(res) != expectedBody {
			http.Error(w, "unexpected request body", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	// Failing server that returns 500
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	// Server that intentionally panics after a partial read
	partialReadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 3)
		n, err := io.ReadFull(r.Body, buf)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
		if n != 3 {
			http.Error(w, "unexpected number of bytes read from request body", http.StatusInternalServerError)
			return
		}
		panic("foo")
	}))
	defer partialReadServer.Close()

	successServerURL, err := url.Parse(successServer.URL)
	c.Assert(err, qt.IsNil)

	failingServerURL, err := url.Parse(failingServer.URL)
	c.Assert(err, qt.IsNil)

	partialReadServerURL, err := url.Parse(partialReadServer.URL)
	c.Assert(err, qt.IsNil)

	tests := []struct {
		description    string
		urls           []*url.URL
		path           string
		statusExpected int
		shouldError    bool
	}{
		{
			description:    "1 working address",
			urls:           []*url.URL{successServerURL},
			statusExpected: http.StatusOK,
		}, {
			description:    "1 working address on a path segment",
			path:           "/foo",
			urls:           []*url.URL{successServerURL},
			statusExpected: http.StatusOK,
		}, {
			description: "2 addresses, first unreachable (connection error), second works",
			urls: []*url.URL{
				{Scheme: "http", Host: "unreachable:61213"},
				successServerURL,
			},
			statusExpected: http.StatusOK,
		}, {
			description: "2 addresses, first returns HTTP 500 (not a transport failure, so no failover)",
			urls: []*url.URL{
				failingServerURL,
				successServerURL,
			},
			statusExpected: http.StatusInternalServerError,
		}, {
			description: "multiple addresses fail with connection errors",
			urls: []*url.URL{
				{Scheme: "https", Host: "unreachable:61213"},
				{Scheme: "https", Host: "another-unreachable:61214"},
			},
			shouldError: true,
		}, {
			description: "server panics after partial read - no retry since request data is lost",
			urls: []*url.URL{
				partialReadServerURL,
				successServerURL,
			},
			shouldError: true,
		},
	}

	for _, test := range tests {
		c.Run(test.description, func(c *qt.C) {
			body := strings.NewReader(expectedBody)
			limitReader := io.LimitReader(body, int64(len(expectedBody))) // Ensure we can only read the data once.
			req, err := http.NewRequest("POST", test.path, limitReader)
			c.Assert(err, qt.IsNil)

			transport, err := rpc.NewMultiBackendTransport(http.DefaultTransport, test.urls)
			c.Assert(err, qt.IsNil)

			resp, err := transport.RoundTrip(req)
			if test.shouldError {
				c.Assert(err, qt.IsNotNil)
				return
			}

			c.Assert(err, qt.IsNil)
			defer resp.Body.Close()
			c.Assert(resp.StatusCode, qt.Equals, test.statusExpected)
		})
	}
}

func TestMultiBackendTransport_NoURLs(t *testing.T) {
	c := qt.New(t)

	// Attempt to create a multiBackendTransport with no URLs should return an error
	transport, err := rpc.NewMultiBackendTransport(http.DefaultTransport, []*url.URL{})
	c.Assert(err, qt.IsNotNil)
	c.Assert(transport, qt.IsNil)
}

// TestMultiBackendTransport_RoundRobin tests the multiBackendTransport
// implementation specifically for round-robin behavior across multiple backends.
func TestMultiBackendTransport_RoundRobin(t *testing.T) {
	c := qt.New(t)

	serverOneCount := atomic.Int32{}
	serverOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverOneCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer serverOne.Close()

	serverTwoCount := atomic.Int32{}
	serverTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverTwoCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer serverTwo.Close()

	serverOneURL, err := url.Parse(serverOne.URL)
	c.Assert(err, qt.IsNil)

	serverTwoURL, err := url.Parse(serverTwo.URL)
	c.Assert(err, qt.IsNil)

	req, err := http.NewRequest("POST", "", nil)
	c.Assert(err, qt.IsNil)

	urls := []*url.URL{serverOneURL, serverTwoURL}
	transport, err := rpc.NewMultiBackendTransport(http.DefaultTransport, urls)
	c.Assert(err, qt.IsNil)

	makeRequest := func() {
		resp, err := transport.RoundTrip(req)
		c.Assert(err, qt.IsNil)
		defer resp.Body.Close()
		c.Assert(resp.StatusCode, qt.Equals, http.StatusOK)
	}

	makeRequest() // First request should go to serverOne
	c.Assert(serverOneCount.Load(), qt.Equals, int32(1))
	c.Assert(serverTwoCount.Load(), qt.Equals, int32(0))

	makeRequest() // Second request should go to serverTwo
	c.Assert(serverOneCount.Load(), qt.Equals, int32(1))
	c.Assert(serverTwoCount.Load(), qt.Equals, int32(1))

	makeRequest() // Then back to serverOne
	c.Assert(serverOneCount.Load(), qt.Equals, int32(2))
	c.Assert(serverTwoCount.Load(), qt.Equals, int32(1))
}
