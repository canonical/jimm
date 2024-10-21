// Copyright 2024 Canonical.
package jimmhttp_test

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/vault"
)

func newStore(t testing.TB) *vault.VaultStore {
	client, path, roleID, roleSecretID, ok := jimmtest.VaultClient(t)
	if !ok {
		t.Skip("vault not available")
	}
	return &vault.VaultStore{
		Client:       client,
		RoleID:       roleID,
		RoleSecretID: roleSecretID,
		KVPath:       path,
	}
}

func getJWKS(c *qt.C) jwk.Set {
	set, err := jwk.ParseString(`
	{
		"keys": [
		  {
			"alg": "RS256",
			"kty": "RSA",
			"use": "sig",
			"n": "yeNlzlub94YgerT030codqEztjfU_S6X4DbDA_iVKkjAWtYfPHDzz_sPCT1Axz6isZdf3lHpq_gYX4Sz-cbe4rjmigxUxr-FgKHQy3HeCdK6hNq9ASQvMK9LBOpXDNn7mei6RZWom4wo3CMvvsY1w8tjtfLb-yQwJPltHxShZq5-ihC9irpLI9xEBTgG12q5lGIFPhTl_7inA1PFK97LuSLnTJzW0bj096v_TMDg7pOWm_zHtF53qbVsI0e3v5nmdKXdFf9BjIARRfVrbxVxiZHjU6zL6jY5QJdh1QCmENoejj_ytspMmGW7yMRxzUqgxcAqOBpVm0b-_mW3HoBdjQ",
			"e": "AQAB",
			"kid": "32d2b213-d3fe-436c-9d4c-67a673890620"
		  }
		]
	}
	`)
	c.Assert(err, qt.IsNil)
	return set
}

func setupWellknownHandlerAndRecorder(c *qt.C, path string, store *vault.VaultStore) *httptest.ResponseRecorder {
	handler := jimmhttp.NewWellKnownHandler(store).Routes()
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", path, nil)
	c.Assert(err, qt.IsNil)
	handler.ServeHTTP(rr, req)
	return rr
}

// 404: In the event the JWKS cannot be found explicitly from
// the credential store.
func TestWellknownAPIJWKSJSONHandles404(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	rr := setupWellknownHandlerAndRecorder(c, "/jwks.json", store)

	resp := rr.Result()
	defer resp.Body.Close()
	code := rr.Code
	b, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(code, qt.Equals, http.StatusNotFound)
	c.Assert(b, qt.JSONEquals, map[string]any{
		"Code":    errors.CodeNotFound,
		"Err":     nil,
		"Message": "JWKS does not exist yet",
		"Op":      "wellknownapi.JWKS",
	})
}

// 500: In the event an expiry cannot be found, but a JWKS can.
func TestWellknownAPIJWKSJSONHandles500(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	jwks := getJWKS(c)

	err = store.PutJWKS(ctx, jwks)
	c.Assert(err, qt.IsNil)

	rr := setupWellknownHandlerAndRecorder(c, "/jwks.json", store)

	resp := rr.Result()
	defer resp.Body.Close()
	code := rr.Code
	b, err := io.ReadAll(resp.Body)

	c.Assert(err, qt.IsNil)
	c.Assert(code, qt.Equals, http.StatusInternalServerError)
	c.Assert(b, qt.JSONEquals, map[string]any{
		"Code":    errors.CodeJWKSRetrievalFailed,
		"Err":     nil,
		"Message": "something went wrong...",
		"Op":      "wellknownapi.JWKS",
	})
}

func TestWellknownAPIJWKSJSONHandles200(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	jwks := getJWKS(c)

	err = store.PutJWKS(ctx, jwks)
	c.Assert(err, qt.IsNil)

	expiry := time.Now().UTC().AddDate(0, 3, 0)
	maxAge := expiry.Sub(time.Now().UTC())
	secondsToExpiry := int64(math.Floor(maxAge.Seconds()))

	err = store.PutJWKSExpiry(ctx, expiry)
	c.Assert(err, qt.IsNil)

	rr := setupWellknownHandlerAndRecorder(c, "/jwks.json", store)

	resp := rr.Result()
	defer resp.Body.Close()
	code := rr.Code
	b, err := io.ReadAll(resp.Body)

	c.Assert(err, qt.IsNil)
	c.Assert(b, qt.JSONEquals, jwks)
	c.Assert(code, qt.Equals, http.StatusOK)
	c.Assert(resp.Header.Get("Cache-Control"), qt.Equals, fmt.Sprintf("must-revalidate, max-age=%d, immutable", secondsToExpiry))
	c.Assert(resp.Header.Get("Expires"), qt.Equals, expiry.Format(time.RFC1123))
}
