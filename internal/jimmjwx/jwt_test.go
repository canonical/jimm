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

func TestBob(t *testing.T) {
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

	jwksService := jimmjwx.NewJWKSService(store)
	startAndTestRotator(c, ctx, store, jwksService)
	jwtService := jimmjwx.NewJWTService(jwksService)
	u, _ := url.Parse(srv.URL)
	jwtService.RegisterJWKSCache(ctx, u.Host, srv.Client())

	// svc, err := jimm.NewService(context.Background(), p)
	// c.Assert(err, qt.IsNil)

	// srv := httptest.NewTLSServer(svc)
	// c.Cleanup(func() { srv.Close() })

	// rr := httptest.NewRecorder()

	// req, err := http.NewRequest("GET", "/.well-known/jwks.json", nil)
	// c.Assert(err, qt.IsNil)
	// svc.ServeHTTP(rr, req)

	// res := rr.Result().Body
	// defer res.Close()
	// body, err := io.ReadAll(res)
	// c.Assert(err, qt.IsNil)

	// fmt.Println(string(body))
	// c.Fail()
}
