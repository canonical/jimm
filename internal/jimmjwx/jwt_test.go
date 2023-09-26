package jimmjwx_test

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/canonical/jimm/internal/jimmjwx"
	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/jwx/v2/jwt"
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
		Secure: true,
		Expiry: time.Minute,
	})

	// Test RegisterJWKSCache does register the public key just setup
	jwtService.RegisterJWKSCache(ctx, srv.Client())

	set, err := jwtService.Cache.Get(ctx, "https://"+u.Host+"/.well-known/jwks.json")
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
		Secure: true,
		Expiry: time.Minute,
	})
	// Setup JWKS Cache
	jwtService.RegisterJWKSCache(ctx, srv.Client())

	// Mint a new JWT
	tok, err := jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@external",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
	})
	c.Assert(err, qt.IsNil)

	// Retrieve pubkey from cache
	set, err := jwtService.Cache.Get(ctx, "https://"+u.Host+"/.well-known/jwks.json")
	c.Assert(err, qt.IsNil)

	// Test the token parses
	token, err := jwt.Parse(
		tok,
		jwt.WithKeySet(set),
	)
	c.Assert(err, qt.IsNil)

	// Test token has what we want
	c.Assert(token.Audience()[0], qt.Equals, "controller-my-diglett-controller")
	c.Assert(token.Subject(), qt.Equals, "diglett@external")
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
		Secure: true,
		Expiry: time.Nanosecond,
	})
	// Setup JWKS Cache
	jwtService.RegisterJWKSCache(ctx, srv.Client())

	// Mint a new JWT
	tok, err := jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@external",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
	})
	c.Assert(err, qt.IsNil)

	// Retrieve pubkey from cache
	set, err := jwtService.Cache.Get(ctx, "https://"+u.Host+"/.well-known/jwks.json")
	c.Assert(err, qt.IsNil)

	time.Sleep(time.Nanosecond * 10)

	// Test the token fails to parse
	_, err = jwt.Parse(
		tok,
		jwt.WithKeySet(set),
	)
	c.Assert(err, qt.ErrorMatches, `"exp" not satisfied`)
}
