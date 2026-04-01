// Copyright 2025 Canonical.

package jimmjwx

import (
	"encoding/json"
	"os"
	"testing"
	"testing/synctest"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

func TestJWTServiceExposesConfiguredJWKS(t *testing.T) {
	c := qt.New(t)
	jwtService, _ := newJWTService(c, time.Minute)

	set, err := jwtService.JWKS.Get(c.Context())
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, 1)
}

func TestNewJWTIsParsableByExponent(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()
	jwtService, set := newJWTService(c, time.Minute)

	// Mint a new JWT
	tok, err := jwtService.NewJWT(ctx, JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@canonical.com",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
		ExtraClaims: map[string]any{
			"my-claim": "my-value",
		},
	})
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
	extraClaim, ok := token.Get("my-claim")
	c.Assert(ok, qt.IsTrue)
	c.Assert(extraClaim, qt.Equals, "my-value")
	c.Assert(token.Issuer(), qt.Equals, "host")
}

func TestNewJWTWithReservedClaimErrors(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()
	jwtService, _ := newJWTService(c, time.Minute)

	_, err := jwtService.NewJWT(ctx, JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@canonical.com",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
		ExtraClaims: map[string]any{
			"access": "foo",
		},
	})
	c.Assert(err, qt.ErrorMatches, `access is a reserved claim`)
}

func TestNewJWTExpires(t *testing.T) {
	c := qt.New(t)
	expiry := time.Second
	ctx := c.Context()
	jwtService, set := newJWTService(c, expiry)

	// Mint a new JWT
	tok, err := jwtService.NewJWT(ctx, JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "diglett@canonical.com",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "administrator",
		},
	})
	c.Assert(err, qt.IsNil)

	// Test the token fails to parse
	_, err = jwt.Parse(
		tok,
		jwt.WithKeySet(set),
		jwt.WithClock(futureClock{expiry: expiry}),
	)
	c.Assert(err, qt.ErrorMatches, `"exp" not satisfied`)
}

func TestNewJWTWithCustomExpiry(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()
	jwtService, set := newJWTService(c, time.Hour)

	shortExpiry := time.Minute // Use a shorter expiry for this token

	// Mint a new JWT with custom expiry
	tok, err := jwtService.NewJWT(ctx, JWTParams{
		Controller: "controller-my-diglett-controller",
		User:       "foo",
		Expiry:     shortExpiry,
	})
	c.Assert(err, qt.IsNil)

	_, err = jwt.Parse(
		tok,
		jwt.WithKeySet(set),
		jwt.WithClock(futureClock{expiry: shortExpiry}),
	)
	c.Assert(err, qt.ErrorMatches, `"exp" not satisfied`)
}

func TestNewJWTUsesRefreshedSigningKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := qt.New(t)
		params, _, _ := newJWKSServiceParams(c)
		service, err := NewJWKSService(c.Context(), params)
		c.Assert(err, qt.IsNil)
		jwtService := NewJWTService(JWTServiceParams{
			Host:   "host",
			Expiry: time.Minute,
			JWKS:   service,
		})
		oldSet := service.cached.set

		refreshedSet, refreshedPrivateKey := generateJWK(c)
		rawJWKS, err := json.Marshal(refreshedSet)
		c.Assert(err, qt.IsNil)
		err = os.WriteFile(params.JWKSPath, rawJWKS, 0o600)
		c.Assert(err, qt.IsNil)
		err = os.WriteFile(params.PrivateKeyPath, refreshedPrivateKey, 0o600)
		c.Assert(err, qt.IsNil)

		time.Sleep(jwksRefreshInterval + time.Minute)

		tok, err := jwtService.NewJWT(c.Context(), JWTParams{
			Controller: "controller-my-diglett-controller",
			User:       "diglett@canonical.com",
		})
		c.Assert(err, qt.IsNil)

		// Check parsing fails with old set
		_, err = jwt.Parse(tok, jwt.WithKeySet(oldSet))
		c.Assert(err, qt.ErrorMatches, `.*failed to find key.*`)

		// Check parsing succeeds with refreshed set
		_, err = jwt.Parse(tok, jwt.WithKeySet(refreshedSet))
		c.Assert(err, qt.IsNil)
	})
}

type futureClock struct {
	expiry time.Duration
}

func (f futureClock) Now() time.Time {
	return time.Now().Add(f.expiry)
}
