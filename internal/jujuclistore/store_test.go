// Copyright 2025 Canonical.

package jujuclistore

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/ulikunitz/xz"
)

// simulates having a xz compressed tar file stream
func makeTarXz(t *testing.T, name string, content []byte) []byte {
	var buf bytes.Buffer

	// XZ compressor
	xzw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("failed to create xz writer: %v", err)
	}

	tw := tar.NewWriter(xzw)

	hdr := &tar.Header{
		Name: name,
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}

	// Close in correct order
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := xzw.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}

	return buf.Bytes()
}

func TestStore(t *testing.T) {
	c := qt.New(t)
	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}

	archive := makeTarXz(t, "juju", []byte("im a juju binary"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, qt.Equals, "/3.6/3.6.2/+download/juju-3.6.2-linux-amd64.tar.xz")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
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
	c.Assert(string(content), qt.Equals, "im a juju binary")
	c.Assert(binary.FullPath, qt.Matches, fmt.Sprintf("%s/juju-.*", os.TempDir()))
}

func TestStoreWithTarMissingJujuBinary(t *testing.T) {
	c := qt.New(t)
	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}

	archive := makeTarXz(t, "diary.txt", []byte("my diary"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, qt.Equals, "/3.6/3.6.2/+download/juju-3.6.2-linux-amd64.tar.xz")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
		c.Assert(err, qt.IsNil)
	}))
	defer server.Close()

	store, err := NewJujuCLIStore(Config{
		BaseURL: server.URL,
	})
	c.Assert(err, qt.IsNil)
	_, err = store.Get(t.Context(), jujuSpec)
	c.Assert(err, qt.ErrorMatches, "juju binary not found in archive")
}

func TestStoreProtectsAgainstDecompressionBomb(t *testing.T) {
	c := qt.New(t)
	jujuSpec := JujuBinarySpec{
		Version: "3.6.2",
		Os:      "linux",
		Arch:    "amd64",
	}

	// Set an extract size 1 byte smaller than the content size
	// to trigger the decompression bomb protection
	c.Patch(&maxExtractSize, int64(9))
	tenBytesContent := []byte("123456789therestofthiswillbemissing") // We'll expect string to missing
	archive := makeTarXz(t, "juju", tenBytesContent)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, qt.Equals, "/3.6/3.6.2/+download/juju-3.6.2-linux-amd64.tar.xz")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
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
	// We expect the content to be truncated to the maxExtractSize
	c.Assert(string(content), qt.Equals, "123456789")
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

	archive := makeTarXz(t, "juju", []byte("im a juju binary"))

	retryCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryCount < 2 {
			retryCount++
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
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
	c.Assert(string(content), qt.Equals, "im a juju binary")
}

func TestStoreCache(t *testing.T) {
	c := qt.New(t)

	archive := makeTarXz(t, "juju", []byte("im a juju binary"))

	called := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
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
	c.Assert(string(content), qt.Equals, "im a juju binary")

	c.Assert(binary.FullPath, qt.Equals, binaryAgain.FullPath) // Ensure the same binary is returned
}

func TestStoreMaxEntriesCleanup(t *testing.T) {
	c := qt.New(t)

	archive := makeTarXz(t, "juju", []byte("im a juju binary"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
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

	archive := makeTarXz(t, "juju", []byte("im a juju binary"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(archive)
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

	err = store.freeEntry() // This should not delete binary1 since its reference count is 1
	c.Assert(err, qt.ErrorMatches, `no entries to delete, max entries limit reached: \d+`)

	c.Assert(len(store.entries), qt.Equals, 2) // Ensure two entries are still kept

	binary1Again.Done()                                          // Mark the first binary as done again
	c.Assert(binary1.referenceCount.Load(), qt.Equals, int32(0)) // Reference count

	// Now the reference count is zero, so it can be deleted
	err = store.freeEntry() // This should delete binary1 since its reference count is 0
	c.Assert(err, qt.IsNil)

	c.Assert(len(store.entries), qt.Equals, 1) // Ensure two entries are still kept
}
