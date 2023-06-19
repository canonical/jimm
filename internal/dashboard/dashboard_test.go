// Copyright 2020 Canonical Ltd.

package dashboard_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/dashboard"
)

const (
	configFile = `var jujuDashboardConfig = {
  // API host to allow app to connect and retrieve models
  baseControllerURL: null,
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

	hnd := dashboard.Handler(context.Background(), "")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusNotFound)
}

func TestDashboardRedirect(t *testing.T) {
	c := qt.New(t)

	hnd := dashboard.Handler(context.Background(), "https://example.com/dashboard")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusPermanentRedirect)
	c.Check(resp.Header.Get("Location"), qt.Equals, "https://example.com/dashboard")
}

func TestDashboardFromPath(t *testing.T) {
	c := qt.New(t)

	dir := c.TempDir()
	err := os.WriteFile(filepath.Join(dir, "config.js.go"), []byte(configFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "test"), []byte(testFile), 0444)
	c.Assert(err, qt.Equals, nil)

	hnd := dashboard.Handler(context.Background(), dir)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusSeeOther)
	c.Check(resp.Header.Get("Location"), qt.Equals, "/")

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp = rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, indexFile)

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/config.js", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp = rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err = io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, `var jujuDashboardConfig = {
  // API host to allow app to connect and retrieve models
  baseControllerURL: null,
  // Configurable base url to allow deploying to different paths.
  baseAppURL: "/",
  // If true then identity will be provided by a third party provider.
  identityProviderAvailable: true,
  // Is this application being rendered in Juju and not JAAS. This flag should
  // only be used for superficial updates like logos. Use feature detection
  // for other environment features.
  isJuju: false,
};
`)

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/test", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp = rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err = io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, testFile)

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/models/alice@external/test", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp = rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err = io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, indexFile)
}

func TestInvalidLocation(t *testing.T) {
	c := qt.New(t)

	hnd := dashboard.Handler(context.Background(), ":::")
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusNotFound)
}

func TestLocationNotDirectory(t *testing.T) {
	c := qt.New(t)

	dir := c.TempDir()
	err := os.WriteFile(filepath.Join(dir, "test"), []byte(testFile), 0444)
	c.Assert(err, qt.Equals, nil)

	hnd := dashboard.Handler(context.Background(), filepath.Join(dir, "test"))
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusNotFound)
}

func TestGUIArchiveEndpoint(t *testing.T) {
	c := qt.New(t)

	dir := c.TempDir()
	err := os.WriteFile(filepath.Join(dir, "config.js.go"), []byte(configFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "test"), []byte(testFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "version.json"), []byte(versionFile), 0444)
	c.Assert(err, qt.Equals, nil)

	hnd := dashboard.Handler(context.Background(), dir)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/gui-archive", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(
		string(buf),
		qt.Equals,
		`{"versions":[{"version":"0.8.1","sha256":"34388e4b0b3e68e2c2ba342cb45f0f21d248fd3c","current":true}]}`,
	)
}
