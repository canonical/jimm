package jimmjwx_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmjwx"
	qt "github.com/frankban/quicktest"
)

func TestRegisterJWKSCacheRegistersTheCacheSuccessfully(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	store := newStore(c)
	// Ensure store is wiped
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	// Setup service for self-call
	p := jimm.Params{
		ControllerUUID:  "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
		VaultAddress:    "http://localhost:8200",
		VaultAuthPath:   "/auth/approle/login",
		VaultPath:       "/jimm-kv/",
		VaultSecretFile: "../../local/vault/approle.json",
	}

	svc, err := jimm.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(func() { srv.Close() })

	// Setup JWKSService
	jwksService := jimmjwx.NewJWKSService(store)
	// Start rotator
	startAndTestRotator(c, ctx, store, jwksService)
	// Setup JWTService
	u, _ := url.Parse(srv.URL)
	jwtService := jimmjwx.NewJWTService(jwksService, u.Host, store)

	// Test RegisterJWKSCache does register the public key just setup
	jwtService.RegisterJWKSCache(ctx, srv.Client())

	set, err := jwtService.Cache.Get(ctx, "https://"+u.Host+"/.well-known/jwks.json")
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, 1)
}
