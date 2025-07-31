// Copyright 2025 Canonical.

package jujuclistore

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

func TestStore(t *testing.T) {
	c := qt.New(t)
	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, qt.Equals, "/3.6/3.6.2/+download/juju-3.6.2-linux-amd64.tar.xz")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("test content"))
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	store, err := NewJujuCLIStore(Config{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)
	binary, err := store.Get(t.Context(), jujuSpec)
	c.Assert(err, qt.IsNil)
	defer binary.Done()

	file, err := os.Open(binary.FullPath)
	c.Assert(err, qt.IsNil)
	content, err := io.ReadAll(file)
	c.Assert(err, qt.IsNil)
	c.Assert(string(content), qt.Equals, "test content")
	c.Assert(binary.FullPath, qt.Matches, fmt.Sprintf("%s/juju-.*", os.TempDir()))
}

func TestStoreError(t *testing.T) {
	c := qt.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetch, err := NewJujuCLIStore(Config{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}
	_, err = fetch.Get(context.Background(), jujuSpec)
	c.Assert(err, qt.ErrorMatches, "request failed with status 404")
}

func TestStoreRetry(t *testing.T) {
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

	store, err := NewJujuCLIStore(Config{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}
	binary, err := store.Get(context.Background(), jujuSpec)
	c.Assert(err, qt.IsNil)
	defer binary.Done()

	file, err := os.Open(binary.FullPath)
	c.Assert(err, qt.IsNil)
	content, err := io.ReadAll(file)
	c.Assert(err, qt.IsNil)
	c.Assert(string(content), qt.Equals, "test content")
}

func TestStoreCache(t *testing.T) {
	c := qt.New(t)
	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("test content"))
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	store, err := NewJujuCLIStore(Config{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}
	binary, err := store.Get(context.Background(), jujuSpec)
	c.Assert(err, qt.IsNil)
	defer binary.Done()

	binaryAgain, err := store.Get(context.Background(), jujuSpec)
	c.Assert(err, qt.IsNil)
	defer binary.Done()

	c.Assert(called, qt.Equals, 1)        // Ensure the server was called only once due to caching
	c.Assert(store.entries, qt.HasLen, 1) // Ensure only one entry is cached

	file, err := os.Open(binaryAgain.FullPath)
	c.Assert(err, qt.IsNil)
	content, err := io.ReadAll(file)
	c.Assert(err, qt.IsNil)
	c.Assert(string(content), qt.Equals, "test content")

	c.Assert(binary.FullPath, qt.Equals, binaryAgain.FullPath) // Ensure the same binary is returned
}

func TestStoreMaxEntriesCleanup(t *testing.T) {
	c := qt.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("test content"))
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	store, err := NewJujuCLIStore(Config{
		BaseURL:    server.URL,
		MaxEntries: 2,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec1 := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}
	binary1, err := store.Get(context.Background(), jujuSpec1)
	c.Assert(err, qt.IsNil)

	jujuSpec2 := JujuBinarySpec{
		Version: "3.7.0",
		Os:      "linux",
		Arch:    "amd64",
	}
	binary2, err := store.Get(context.Background(), jujuSpec2)
	c.Assert(err, qt.IsNil)
	defer binary2.Done()
	c.Assert(len(store.entries), qt.Equals, 2) // Ensure only two entries are kept

	jujuSpec3 := JujuBinarySpec{
		Version: "3.7.1",
		Os:      "linux",
		Arch:    "amd64",
	}
	_, err = store.Get(context.Background(), jujuSpec3)
	c.Assert(err, qt.ErrorMatches, `no entries to delete, max entries limit reached: \d+`)
	c.Assert(len(store.entries), qt.Equals, 2) // Ensure still only two entries are kept

	binary1.Done() // Mark the first binary as done

	binary3, err := store.Get(context.Background(), jujuSpec3)
	c.Assert(err, qt.IsNil)
	defer binary3.Done()
	c.Assert(len(store.entries), qt.Equals, 2)

	binary3Again, err := store.Get(context.Background(), jujuSpec3)
	c.Assert(err, qt.IsNil)
	defer binary3Again.Done()

	c.Assert(binary3.FullPath, qt.Equals, binary3Again.FullPath) // Ensure the same binary is returned
}

func TestStoreReferenceCount(t *testing.T) {
	c := qt.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("test content"))
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	store, err := NewJujuCLIStore(Config{
		BaseURL:    server.URL,
		MaxEntries: 2,
	})
	c.Assert(err, qt.IsNil)

	jujuSpec1 := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}
	binary1, err := store.Get(context.Background(), jujuSpec1)
	c.Assert(err, qt.IsNil)

	binary1Again, err := store.Get(context.Background(), jujuSpec1)
	c.Assert(err, qt.IsNil)

	c.Assert(binary1.FullPath, qt.Equals, binary1Again.FullPath) // Ensure the same binary is returned

	jujuSpec2 := JujuBinarySpec{
		Version: "3.7.0",
		Os:      "linux",
		Arch:    "amd64",
	}
	_, err = store.Get(context.Background(), jujuSpec2)
	c.Assert(err, qt.IsNil)

	// Cache is now full, the reference count for binary1 should be 2.
	c.Assert(binary1.referenceCount.Load(), qt.Equals, int32(2))

	binary1.Done()                                               // Mark the first binary as done
	c.Assert(binary1.referenceCount.Load(), qt.Equals, int32(1)) // Reference count

	err = store.freeEntry(context.Background()) // This should not delete binary1 since its reference count is 1
	c.Assert(err, qt.ErrorMatches, `no entries to delete, max entries limit reached: \d+`)

	c.Assert(len(store.entries), qt.Equals, 2) // Ensure two entries are still kept

	binary1Again.Done()                                          // Mark the first binary as done again
	c.Assert(binary1.referenceCount.Load(), qt.Equals, int32(0)) // Reference count

	// Now the reference count is zero, so it can be deleted
	err = store.freeEntry(context.Background()) // This should not delete binary1 since its reference count is 1
	c.Assert(err, qt.IsNil)

	c.Assert(len(store.entries), qt.Equals, 1) // Ensure two entries are still kept
}
