// Copyright 2025 Canonical.

package jujuclifetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFetch(t *testing.T) {
	c := qt.New(t)
	jujuSpec := JujuBinarySpec{
		VersionWithMinor: "3.6",
		VersionWithPatch: "3.6.2",
		Os:               "linux",
		Arch:             "amd64",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, qt.Equals, "/3.6/3.6.2/+download/juju-3.6.2-linux-amd64.tar.xz")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("test content"))
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	fetcher, err := NewJujuCLIFetcher(jujuCLIFetcherConfig{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)
	binary, err := fetcher.Fetch(context.Background(), jujuSpec)
	c.Assert(err, qt.IsNil)
	defer func() {
		err = binary.Remove()
		c.Assert(err, qt.IsNil)
	}()

	file, err := os.Open(binary.FullPath)
	c.Assert(err, qt.IsNil)
	content, err := io.ReadAll(file)
	c.Assert(err, qt.IsNil)
	c.Assert(string(content), qt.Equals, "test content")
	c.Assert(binary.FullPath, qt.Matches, fmt.Sprintf("%s/juju-.*", os.TempDir()))
}

func TestFetchError(t *testing.T) {
	c := qt.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetch, err := NewJujuCLIFetcher(jujuCLIFetcherConfig{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec := JujuBinarySpec{
		VersionWithMinor: "3.6",
		VersionWithPatch: "3.6.2",
		Os:               "linux",
		Arch:             "amd64",
	}
	_, err = fetch.Fetch(context.Background(), jujuSpec)
	c.Assert(err, qt.ErrorMatches, "request failed with status 404")
}

func TestFetchRetry(t *testing.T) {
	c := qt.New(t)
	retryCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryCount < 2 {
			retryCount++
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("test content"))
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	fetcher, err := NewJujuCLIFetcher(jujuCLIFetcherConfig{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec := JujuBinarySpec{
		VersionWithMinor: "3.6",
		VersionWithPatch: "3.6.2",
		Os:               "linux",
		Arch:             "amd64",
	}
	binary, err := fetcher.Fetch(context.Background(), jujuSpec)
	c.Assert(err, qt.IsNil)
	defer func() {
		err = binary.Remove()
		c.Assert(err, qt.IsNil)
	}()

	file, err := os.Open(binary.FullPath)
	c.Assert(err, qt.IsNil)
	content, err := io.ReadAll(file)
	c.Assert(err, qt.IsNil)
	c.Assert(string(content), qt.Equals, "test content")
}
