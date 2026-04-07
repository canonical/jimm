// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type failingJWKSProvider struct{}

func (failingJWKSProvider) Get(context.Context) (jwk.Set, error) {
	return nil, stderrors.New("boom")
}

func newJWKSService(c *qt.C) (*jimmjwx.JWKSService, jimmjwx.JWKSServiceParams) {
	params, err := jimmtest.StaticJWKSServiceParams(c)
	c.Assert(err, qt.IsNil)
	service, err := jimmjwx.NewJWKSService(c.Context(), params)
	c.Assert(err, qt.IsNil)
	return service, params
}

func newMultiKeyJWKSService(c *qt.C) (*jimmjwx.JWKSService, string) {
	_, params := newJWKSService(c)
	var document map[string][]map[string]any
	rawJWKS, err := os.ReadFile(params.JWKSPath)
	c.Assert(err, qt.IsNil)
	err = json.Unmarshal(rawJWKS, &document)
	c.Assert(err, qt.IsNil)

	duplicateKey := make(map[string]any, len(document["keys"][0]))
	maps.Copy(duplicateKey, document["keys"][0])
	duplicateKey["kid"] = "old-test-kid"
	document["keys"] = append(document["keys"], duplicateKey)

	rawJWKS, err = json.Marshal(document)
	c.Assert(err, qt.IsNil)
	err = os.WriteFile(params.JWKSPath, rawJWKS, 0o600)
	c.Assert(err, qt.IsNil)
	service, err := jimmjwx.NewJWKSService(c.Context(), params)
	c.Assert(err, qt.IsNil)
	return service, string(rawJWKS)
}

func setupWellknownHandlerAndRecorder(c *qt.C, path string, provider jimmhttp.JWKSProvider) *httptest.ResponseRecorder {
	handler := jimmhttp.NewWellKnownHandler(provider).Routes()
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", path, nil)
	c.Assert(err, qt.IsNil)
	handler.ServeHTTP(rr, req)
	return rr
}

func assertJSONBodyEquals(c *qt.C, body []byte, expected string) {
	c.Helper()
	var got any
	var want any
	err := json.Unmarshal(body, &got)
	c.Assert(err, qt.IsNil)
	err = json.Unmarshal([]byte(expected), &want)
	c.Assert(err, qt.IsNil)
	c.Assert(got, qt.DeepEquals, want)
}

func TestWellknownAPIJWKSJSONHandles500(t *testing.T) {
	c := qt.New(t)
	rr := setupWellknownHandlerAndRecorder(c, "/jwks.json", failingJWKSProvider{})

	resp := rr.Result()
	defer resp.Body.Close()
	code := rr.Code
	b, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(code, qt.Equals, http.StatusInternalServerError)
	c.Assert(b, qt.JSONEquals, map[string]any{
		"Code":    errors.CodeJWKSRetrievalFailed,
		"Err":     nil,
		"Info":    nil,
		"Message": "failed to retrieve JWKS",
	})
}

func TestWellknownAPIJWKSJSONHandles200(t *testing.T) {
	c := qt.New(t)
	service, params := newJWKSService(c)
	rr := setupWellknownHandlerAndRecorder(c, "/jwks.json", service)

	resp := rr.Result()
	defer resp.Body.Close()
	code := rr.Code
	b, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(code, qt.Equals, http.StatusOK)
	rawJWKS, err := os.ReadFile(params.JWKSPath)
	c.Assert(err, qt.IsNil)
	assertJSONBodyEquals(c, b, string(rawJWKS))
	c.Assert(resp.Header.Get("Cache-Control"), qt.Equals, "max-age=600")
}

func TestWellknownAPIJWKSJSONServesMultipleKeys(t *testing.T) {
	c := qt.New(t)
	service, rawJWKS := newMultiKeyJWKSService(c)
	rr := setupWellknownHandlerAndRecorder(c, "/jwks.json", service)

	resp := rr.Result()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(rr.Code, qt.Equals, http.StatusOK)
	assertJSONBodyEquals(c, b, rawJWKS)

	var body struct {
		Keys []map[string]any `json:"keys"`
	}
	err = json.Unmarshal(b, &body)
	c.Assert(err, qt.IsNil)
	c.Assert(body.Keys, qt.HasLen, 2)
}
