package jimmjwx_test

import (
	"context"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/internal/jimmjwx"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestGenerateJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jwks, privKeyPem, err := jimmjwx.GenerateJWK(ctx)
	c.Assert(err, qt.IsNil)

	jwksIter := jwks.Keys(ctx)
	jwksIter.Next(ctx)
	key := jwksIter.Pair().Value.(jwk.Key)

	// kid
	_, err = uuid.Parse(key.KeyID())
	c.Assert(err, qt.IsNil)
	// use
	c.Assert(key.KeyUsage(), qt.Equals, "sig")
	// alg
	c.Assert(key.Algorithm(), qt.Equals, jwa.RS256)

	// It's fine for us to just test the key exists.
	c.Assert(string(privKeyPem), qt.Contains, "-----BEGIN RSA PRIVATE KEY-----")
}

// This test is difficult to gauge, as it is truly only time based.
// As such, it will retry 60 times on a 500ms basis.
func TestStartJWKSRotatorWithNoJWKSInTheStore(t *testing.T) {
	c := qt.New(t)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)
	svc := jimmjwx.NewJWKSService(store)
	startAndTestRotator(c, ctx, store, svc)
}

// Due to the nature of this test, we do not test exact times (as it will vary drastically machine to machine)
// But rather just ensure the JWKS has infact updated.
//
// So I suppose this test is "best effort", but will only ever pass if the code is truly OK.
func TestStartJWKSRotatorRotatesAJWKS(t *testing.T) {
	c := qt.New(t)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	svc := jimmjwx.NewJWKSService(store)

	// So, we first put a fresh JWKS in the store
	err = store.PutJWKS(ctx, getJWKS(c))
	c.Check(err, qt.IsNil)

	// Get the key we're aware of right now
	ks, err := store.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	initialKey, ok := ks.Key(0)
	c.Assert(ok, qt.IsTrue)

	// Start up the rotator
	err = svc.StartJWKSRotator(ctx, time.NewTicker(time.Second).C, time.Now())
	c.Assert(err, qt.IsNil)

	// We retry 500ms * 60 (30s) to test the diff
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		ks2, err := store.GetJWKS(ctx)
		c.Assert(err, qt.IsNil)
		newKey, ok := ks2.Key(0)
		c.Assert(ok, qt.IsTrue)
		if initialKey.KeyID() == newKey.KeyID() {
			break
		}
	}
}
