// Copyright 2025 Canonical.

package jujuclifetcher

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/juju/clock"
	jujuerrrors "github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// launchPadURL is the base URL for the Juju binary downloads.
const launchPadURL = "https://launchpad.net/juju"

// launchPadTemplate is the template for constructing the download URL for Juju binaries.
const launchPadTemplate = "{{.BaseURL}}/{{.VersionWithMinor}}/{{.VersionWithPatch}}/+download/juju-{{.VersionWithPatch}}-{{.Os}}-{{.Arch}}.tar.xz"

var retryRequestError = jujuerrrors.New("retry request error")

// jujuCLIFetcherConfig holds the configuration for the Juju binary fetcher.
type jujuCLIFetcherConfig struct {
	BaseURL string // Base URL for the Juju binary downloads. Example: "https://launchpad.net/juju"
	Dir     string // Directory to store the downloaded binaries. Defaults to the system's temp directory.
}

// JujuBinarySpec defines the specifications for a Juju binary to be downloaded.
type JujuBinarySpec struct {
	VersionWithMinor string // Version with minor version number, e.g., "3.6"
	VersionWithPatch string // Version with patch version number, e.g., "3.6.2"
	Os               string // Operating system, e.g., "linux"
	Arch             string // Architecture, e.g., "amd64"
}

type jujuCLIFetcher struct {
	config   jujuCLIFetcherConfig
	template template.Template
}

// NewJujuCLIFetcher creates a new jujuCLIFetcher instance with the provided configuration.
// If the BaseURL is not provided, it defaults to the launchpad URL.
func NewJujuCLIFetcher(cfg jujuCLIFetcherConfig) (*jujuCLIFetcher, error) {
	if cfg.BaseURL == "" {
		// Default to the launchpad URL if no base URL is provided.
		cfg.BaseURL = launchPadURL
	}
	tmpl, err := template.New("URL").Parse(launchPadTemplate)
	if err != nil {
		return nil, err
	}
	return &jujuCLIFetcher{
		config:   cfg,
		template: *tmpl,
	}, nil
}

// Binary represents a downloaded Juju binary.
// It contains the full path to the binary file.
// It also provides a method to clean up the binary file.
type Binary struct {
	FullPath string
}

// Remove removes the binary file from the filesystem.
// It returns an error if the file cannot be removed.
func (b Binary) Remove() error {
	return os.Remove(b.FullPath)
}

// Fetch downloads the Juju binary specified by the JujuBinarySpec.
// It returns a Binary instance containing the full path to the downloaded binary.
// If the download fails, it returns an error.
// It retries the download on server errors or rate limiting.
// The retry logic uses exponential backoff.
// The context can be used to cancel the operation.
func (p jujuCLIFetcher) Fetch(ctx context.Context, spec JujuBinarySpec) (*Binary, error) {
	var buf bytes.Buffer
	err := p.template.Execute(&buf, map[string]string{
		"BaseURL":          p.config.BaseURL,
		"VersionWithMinor": spec.VersionWithMinor,
		"VersionWithPatch": spec.VersionWithPatch,
		"Os":               spec.Os,
		"Arch":             spec.Arch,
	})
	if err != nil {
		return nil, err
	}
	var file *os.File
	err = retry.Call(retry.CallArgs{
		Func: func() error {
			resp, err := http.Get(buf.String())
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			// Retry on server errors or rate limiting.
			if (resp.StatusCode >= 500 && resp.StatusCode < 600) || resp.StatusCode == http.StatusTooManyRequests {
				return retryRequestError
			}
			if resp.StatusCode != http.StatusOK {
				return jujuerrrors.Errorf("request failed with status %d", resp.StatusCode)
			}
			file, err = os.CreateTemp(p.config.Dir, "juju-*")
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(file, resp.Body)
			if err != nil {
				return err
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			return !jujuerrrors.Is(err, retryRequestError)
		},
		BackoffFunc: retry.DoubleDelay,
		Attempts:    10,
		Delay:       time.Second,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})
	if err != nil {
		if file != nil {
			// Clean up the file if it was created.
			// We ignore the error because there is not much we can do about it.
			errRemove := os.Remove(file.Name())
			if errRemove != nil {
				zapctx.Error(ctx, "failed to remove temporary file", zap.Error(errRemove), zap.String("file", file.Name()))
			}
		}
		return nil, err
	}
	defer file.Close()

	return &Binary{
		FullPath: file.Name(),
	}, nil
}
