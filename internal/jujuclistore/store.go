// Copyright 2025 Canonical.

package jujuclistore

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"github.com/ulikunitz/xz"
	"go.uber.org/zap"
)

const timeoutRequestDuration = 5 * time.Minute

// launchPadURL is the base URL for the Juju binary downloads.
const launchPadURL = "https://launchpad.net/juju"

// launchPadTemplate is the template for constructing the download URL for Juju binaries.
const launchPadTemplate = "{{.BaseURL}}/{{.VersionWithMinor}}/{{.VersionWithPatch}}/+download/juju-{{.VersionWithPatch}}-{{.Os}}-{{.Arch}}.tar.xz"

var (
	retryRequestError error = jujuerrors.New("retry request error")
	maxExtractSize    int64 = 200 * 1024 * 1024
)

// Config holds the configuration for the Juju binary fetcher.
type Config struct {
	// Base URL for the Juju binary downloads. Example: "https://launchpad.net/juju"
	BaseURL string
	// Directory to store the downloaded binaries. Defaults to the system's temp directory.
	Dir string
	// Maximum number of entries to keep in the directory. Defaults to 2.
	MaxEntries int
}

// JujuBinarySpec defines the specifications for a Juju binary to be downloaded.
type JujuBinarySpec struct {
	// Version with patch version number, e.g., "3.6.2"
	Version string
	// Operating system, e.g., "linux"
	Os string
	// Architecture, e.g., "amd64"
	Arch string
}

type jujuCLIStore struct {
	config   Config
	template template.Template
	// HTTP client for making requests
	client *http.Client

	// Protects the entries map
	lock sync.Mutex
	// Map to keep track of downloaded entries
	entries map[string]*Binary
}

// NewJujuCLIStore creates a new jujuCLIFetcher instance with the provided configuration.
// If the BaseURL is not provided, it defaults to the launchpad URL.
func NewJujuCLIStore(cfg Config) (*jujuCLIStore, error) {
	if cfg.BaseURL == "" {
		// Default to the launchpad URL if no base URL is provided.
		cfg.BaseURL = launchPadURL
	}
	tmpl, err := template.New("URL").Parse(launchPadTemplate)
	if err != nil {
		return nil, err
	}
	if cfg.MaxEntries <= 0 {
		// Default to 2 entries if MaxEntries is not set or is less than or
		// equal to zero.
		cfg.MaxEntries = 2
	}
	if cfg.MaxEntries > 10 {
		// Limit the maximum number of entries to 10 to prevent excessive memory usage.
		return nil, jujuerrors.Errorf("max entries limit is too high: %d, must be <= 10", cfg.MaxEntries)
	}
	return &jujuCLIStore{
		config:   cfg,
		template: *tmpl,
		client:   &http.Client{Timeout: timeoutRequestDuration},
		entries:  make(map[string]*Binary, cfg.MaxEntries),
		lock:     sync.Mutex{},
	}, nil
}

// Binary represents a downloaded Juju binary.
// It contains the full path to the binary file.
// It also provides a method to mark the binary as done, which can be used to indicate that the binary
// is no longer used.
type Binary struct {
	FullPath string

	referenceCount atomic.Int32
}

// Done marks the binary as done by decrementing its reference count.
// This method should be called when the binary is no longer needed.
// If the reference count reaches zero, it indicates that the binary can be deleted.
func (b *Binary) Done() {
	b.referenceCount.Add(-1)
}

// Get downloads the Juju binary specified by the JujuBinarySpec.
// It returns a Binary instance containing the full path to the downloaded binary.
// If the download fails, it returns an error.
// It retries the download on server errors or rate limiting.
// The retry logic uses exponential backoff.
// The context can be used to cancel the operation.
func (p *jujuCLIStore) Get(ctx context.Context, spec JujuBinarySpec) (*Binary, error) {
	var buf bytes.Buffer

	v, err := version.Parse(spec.Version)
	if err != nil {
		return nil, jujuerrors.Annotatef(err, "invalid version %q", spec.Version)
	}
	err = p.template.Execute(&buf, map[string]string{
		"BaseURL":          p.config.BaseURL,
		"VersionWithMinor": fmt.Sprintf("%d.%d", v.Major, v.Minor),
		"VersionWithPatch": fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch),
		"Os":               spec.Os,
		"Arch":             spec.Arch,
	})
	if err != nil {
		return nil, err
	}
	url := buf.String()
	zapctx.Debug(
		ctx,
		"getting Juju binary",
		zap.String("version", spec.Version),
		zap.String("os", spec.Os),
		zap.String("arch", spec.Arch),
		zap.String("url", url),
	)

	// worst case the lock will be help for the duration of the download
	// of the binary. For now it is acceptable because we support bootstrapping
	// one controller at a time.
	p.lock.Lock()
	defer p.lock.Unlock()
	binary, ok := p.entries[url]
	if ok {
		binary.referenceCount.Add(1) // Increment reference count
		return binary, nil
	}
	err = p.freeEntry()
	if err != nil {
		return nil, err
	}
	var file *os.File

	err = retry.Call(retry.CallArgs{
		Func: func() error {
			file, err = p.downloadFile(ctx, url)
			if err != nil {
				return err
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			return !jujuerrors.Is(err, retryRequestError)
		},
		BackoffFunc: retry.DoubleDelay,
		Attempts:    10,
		Delay:       time.Second,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})
	if err != nil {
		return nil, err
	}

	binary = &Binary{
		FullPath: file.Name(),
	}
	binary.referenceCount.Store(1)
	p.entries[url] = binary
	return binary, nil
}

// freeEntry checks if the entries map has reached the maximum number of entries.
// If it has, it deletes a random entry from the map.
// It should be called when the entries map is locked to ensure thread safety.
func (p *jujuCLIStore) freeEntry() error {
	if len(p.entries) < p.config.MaxEntries {
		return nil
	}
	entriesToDelete := []string{}
	for key, binary := range p.entries {
		if binary.referenceCount.Load() == 0 {
			entriesToDelete = append(entriesToDelete, key)
		}
	}

	if len(entriesToDelete) == 0 {
		// This shouldn't currently happen because we bootstrap one controller at a time,
		// so there should always be at least one
		return jujuerrors.Errorf("no entries to delete, max entries limit reached: %d", p.config.MaxEntries)
	}
	//nolint:gosec
	// G404: Use of weak random number generator is acceptable here for cache eviction
	entryToDelete := entriesToDelete[rand.IntN(len(entriesToDelete))]
	binary := p.entries[entryToDelete]
	err := os.Remove(binary.FullPath)
	if err != nil {
		return jujuerrors.Annotatef(err, "failed to remove binary at path %s", binary.FullPath)
	} else {
		delete(p.entries, entryToDelete)
	}

	return nil
}

// downloadFile downloads the file from the specified URL and returns a file handle.
// It retries on server errors or rate limiting.
// If the download fails, it returns an error.
// The file is created in the directory specified in the configuration.
// The context can be used to cancel the operation.
func (p *jujuCLIStore) downloadFile(ctx context.Context, downloadUrl string) (*os.File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if (resp.StatusCode >= 500 && resp.StatusCode < 600) || resp.StatusCode == http.StatusTooManyRequests {
		return nil, retryRequestError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, jujuerrors.Errorf("request failed with status %d", resp.StatusCode)
	}

	xzReader, err := xz.NewReader(resp.Body)
	if err != nil {
		return nil, jujuerrors.Annotatef(err, "failed to create xz reader for %s", downloadUrl)
	}
	tarReader := tar.NewReader(xzReader)
	// 200mb - limit download size should malicious actor send large file
	// and destroy jimm's disk with decompression bomb.

	limitedReader := io.LimitReader(tarReader, maxExtractSize)

	var binaryFile *os.File
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Skip non-regular files and non-binary files
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		zapctx.Debug(ctx, "processing tar header", zap.String("name", hdr.Name))
		if filepath.Base(hdr.Name) != "juju" {
			continue
		}

		// Create a temp file for the binary
		f, err := os.CreateTemp(p.config.Dir, "juju-*")
		if err != nil {
			return nil, err
		}

		if _, err := io.Copy(f, limitedReader); err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, err
		}

		if err := f.Chmod(0755); err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, jujuerrors.Annotatef(err, "failed to set permissions on binary file %s", f.Name())
		}
		binaryFile = f
		break
	}

	if binaryFile == nil {
		return nil, fmt.Errorf("juju binary not found in archive")
	}

	if err := binaryFile.Close(); err != nil {
		return nil, jujuerrors.Annotatef(err, "failed to close binary file %s", binaryFile.Name())
	}

	return binaryFile, nil
}
