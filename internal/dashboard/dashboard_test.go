// Copyright 2020 Canonical Ltd.

package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dashboard"
)

const (
	configFile = `var jujuDashboardConfig = {
  // API host to allow app to connect and retrieve models
  baseControllerURL: "{{.baseControllerURL}}",
  // Configurable base url to allow deploying to different paths.
  baseAppURL: "{{.baseAppURL}}",
  // If true then identity will be provided by a third party provider.
  identityProviderAvailable: {{.identityProviderAvailable}},
  // Is this application being rendered in Juju and not JAAS. This flag should
  // only be used for superficial updates like logos. Use feature detection
  // for other environment features.
  isJuju: {{.isJuju}},
};
`
	versionFile = `{"version": "0.8.1", "git-sha": "34388e4b0b3e68e2c2ba342cb45f0f21d248fd3c"}`
	indexFile   = `Index File`
	testFile    = `TEST`
)

func TestDashboardNotConfigured(t *testing.T) {
	c := qt.New(t)

	hnd := dashboard.Handler(context.Background(), "", "")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusNotFound)
}

func TestDashboardRedirect(t *testing.T) {
	c := qt.New(t)

	hnd := dashboard.Handler(context.Background(), "https://example.com/dashboard", "http://jimm.canonical.com")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusPermanentRedirect)
	c.Check(resp.Header.Get("Location"), qt.Equals, "https://example.com/dashboard")
}

func TestInvalidLocation(t *testing.T) {
	c := qt.New(t)

	hnd := dashboard.Handler(context.Background(), ":::", "http://jimm.canonical.com")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusNotFound)
}

func TestLocationNotDirectory(t *testing.T) {
	c := qt.New(t)

	dir := c.TempDir()
	err := os.WriteFile(filepath.Join(dir, "test"), []byte(testFile), 0600)
	c.Assert(err, qt.Equals, nil)

	hnd := dashboard.Handler(context.Background(), filepath.Join(dir, "test"), "http://jimm.canonical.com")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusNotFound)
}
