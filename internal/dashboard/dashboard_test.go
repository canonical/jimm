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
	configFile = `// eslint-disable-next-line no-unused-vars
var jaasDashboardConfig = {
  // API host to allow app to connect and retrieve models
  baseControllerURL: "jimm.jujucharms.com",
  // Configurable base url to allow deploying to different paths.
  baseAppURL: "/dashboard",
  // If true then identity will be provided by a third party provider.
  identityProviderAvailable: true
};
`
	indexFile = `Index File`
	testFile  = `TEST`
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

	dir := c.Mkdir()
	err := os.WriteFile(filepath.Join(dir, "config.js.go"), []byte(configFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexFile), 0444)
	c.Assert(err, qt.Equals, nil)
	err = os.WriteFile(filepath.Join(dir, "test"), []byte(testFile), 0444)
	c.Assert(err, qt.Equals, nil)

	hnd := dashboard.Handler(context.Background(), dir)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard/", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, indexFile)

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/dashboard/config.js", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp = rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err = io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, `// eslint-disable-next-line no-unused-vars
var jaasDashboardConfig = {
  // API host to allow app to connect and retrieve models
  baseControllerURL: "jimm.jujucharms.com",
  // Configurable base url to allow deploying to different paths.
  baseAppURL: "/dashboard",
  // If true then identity will be provided by a third party provider.
  identityProviderAvailable: true
};
`)

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/dashboard/test", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp = rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err = io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, testFile)
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

	dir := c.Mkdir()
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

func TestNoTemplate(t *testing.T) {
	c := qt.New(t)

	dir := c.Mkdir()
	err := os.WriteFile(filepath.Join(dir, "config.js"), []byte(testFile), 0444)
	c.Assert(err, qt.Equals, nil)

	hnd := dashboard.Handler(context.Background(), dir)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/dashboard/config.js", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, testFile)
}
