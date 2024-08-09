// Copyright 2024 Canonical.
package jimmjwx_test

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/iter/arrayiter"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

func TestRegisterJWKSCacheRegistersTheCacheSuccessfully(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	_, srv, store := setupService(ctx, c)

	// Setup JWKSService
	jwksService := jimmjwx.NewJWKSService(store)
	// Start rotator
	startAndTestRotator(c, ctx, store, jwksService)
	// Setup JWTService
	u, _ := url.Parse(srv.URL)
	jwtService := jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   u.Host,
		Store:  store,
		Expiry: time.Minute,
	})

	set, err := jwtService.JWKS.Get(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, 1)
}

func TestNewJWTIsParsableByExponent(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set env for 'fake' service
	c.Assert(os.Setenv("JIMM_DNS_NAME", "127.0.0.1:17070"), qt.IsNil)
	c.Assert(os.Setenv("JIMM_JWT_EXPIRY", "10s"), qt.IsNil)
	c.Cleanup(func() {
		os.Clearenv()
	})

	_, srv, store := setupService(ctx, c)

	// Setup JWKSService
	jwksService := jimmjwx.NewJWKSService(store)
	// Start rotator
	startAndTestRotator(c, ctx, store, jwksService)
	// Setup JWTService
	u, _ := url.Parse(srv.URL)
	jwtService := jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   u.Host,
		Store:  store,
		Expiry: time.Minute,
	})

	// Mint a new JWT
	tok, err := jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@canonical.com",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
	})
	c.Assert(err, qt.IsNil)

	// Retrieve pubkey from cache
	set, err := jwtService.JWKS.Get(ctx)
	c.Assert(err, qt.IsNil)

	// Test the token parses
	token, err := jwt.Parse(
		tok,
		jwt.WithKeySet(set),
	)
	c.Assert(err, qt.IsNil)

	// Test token has what we want
	c.Assert(token.Audience()[0], qt.Equals, "controller-my-diglett-controller")
	c.Assert(token.Subject(), qt.Equals, "diglett@canonical.com")
	accessClaim, ok := token.Get("access")
	c.Assert(ok, qt.IsTrue)
	c.Assert(accessClaim, qt.DeepEquals, map[string]any{
		"controller": "superuser",
		"model":      "administrator",
	})
	c.Assert(token.Issuer(), qt.Equals, u.Host)
}

func TestNewJWTExpires(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set env for 'fake' service
	c.Assert(os.Setenv("JIMM_DNS_NAME", "127.0.0.1:17070"), qt.IsNil)
	c.Assert(os.Setenv("JIMM_JWT_EXPIRY", "1ns"), qt.IsNil)
	c.Cleanup(func() {
		os.Clearenv()
	})

	_, srv, store := setupService(ctx, c)

	// Setup JWKSService
	jwksService := jimmjwx.NewJWKSService(store)
	// Start rotator
	startAndTestRotator(c, ctx, store, jwksService)
	// Setup JWTService
	u, _ := url.Parse(srv.URL)
	jwtService := jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   u.Host,
		Store:  store,
		Expiry: time.Nanosecond,
	})

	// Mint a new JWT
	tok, err := jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@canonical.com",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
	})
	c.Assert(err, qt.IsNil)

	// Retrieve pubkey from cache
	set, err := jwtService.JWKS.Get(ctx)
	c.Assert(err, qt.IsNil)

	time.Sleep(time.Nanosecond * 10)

	// Test the token fails to parse
	_, err = jwt.Parse(
		tok,
		jwt.WithKeySet(set),
	)
	c.Assert(err, qt.ErrorMatches, `"exp" not satisfied`)
}

func TestCredentialCache(t *testing.T) {
	c := qt.New(t)
	store := newStore(c)
	ctx := context.Background()
	set, _, err := jimmjwx.GenerateJWK(ctx)
	c.Assert(err, qt.IsNil)
	err = store.PutJWKS(ctx, set)
	c.Assert(err, qt.IsNil)
	vaultCache := jimmjwx.NewCredentialCache(store)
	gotSet, err := vaultCache.Get(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(gotSet.Len(), qt.Not(qt.Equals), 0)
	expectedKeyPairs := getKeyPairs(ctx, set)
	wantKeyPairs := getKeyPairs(ctx, gotSet)
	c.Assert(expectedKeyPairs, qt.DeepEquals, wantKeyPairs)
}

func getKeyPairs(ctx context.Context, set jwk.Set) []*arrayiter.Pair {
	res := make([]*arrayiter.Pair, 0)
	iterator := set.Keys(ctx)
	for val := iterator.Pair(); iterator.Next(ctx); val = iterator.Pair() {
		res = append(res, val)
	}
	return res
}
