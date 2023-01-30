package jimmjwx

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/vault"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func newStore(t testing.TB) *vault.VaultStore {
	client, path, creds, ok := jimmtest.VaultClient(t, "../../")
	if !ok {
		t.Skip("vault not available")
	}
	return &vault.VaultStore{
		Client:     client,
		AuthSecret: creds,
		AuthPath:   "/auth/approle/login",
		KVPath:     path,
	}
}

func resetJWKS(c *qt.C, store *vault.VaultStore) {
	client := store.Client
	client.SetToken("token")
	wellKnownPath := path.Join(store.KVPath, "creds", ".well-known")
	jwksJsonPath := path.Join(wellKnownPath, "jwks.json")
	jwksKeyPath := path.Join(wellKnownPath, "jwks-key.pem")
	jwkExpiryPath := path.Join(wellKnownPath, "jwks-expiry")
	_, err := client.Logical().Delete(jwkExpiryPath)
	c.Check(err, qt.IsNil)
	_, err = client.Logical().Delete(jwksJsonPath)
	c.Check(err, qt.IsNil)
	_, err = client.Logical().Delete(jwksKeyPath)
	c.Check(err, qt.IsNil)
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

func TestGenerateJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	svc := NewJWKSService(store)

	jwks, privKeyPem, err := svc.generateJWK(ctx)
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
// As such we take a -3 deficit to our total suites test time.
// But this is a fair usecase for time-based-testing.
func TestStartJWKSRotatorWithNoJWKSInTheStore(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	resetJWKS(c, store)
	svc := NewJWKSService(store)

	stopRotator, err := svc.StartJWKSRotator(ctx, time.NewTicker(time.Second), time.Now())
	c.Assert(err, qt.IsNil)
	time.Sleep(time.Second)
	stopRotator()
	ks, err := store.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)

	ki := ks.Keys(ctx)
	ki.Next(ctx)
	key := ki.Pair().Value.(jwk.Key)
	_, err = uuid.Parse(key.KeyID())
	c.Assert(err, qt.IsNil)
}

// Due to the nature of this test, we do not test exact times (as it will vary drastically machine to machine)
// But rather just ensure the JWKS has infact updated.
//
// So I suppose this test is "best effort", but will only ever pass if the code is truly OK.
func TestStartJWKSRotatorRotatesAJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	resetJWKS(c, store)
	svc := NewJWKSService(store)

	// So, we first put a fresh JWKS in the store
	err := store.PutJWKS(ctx, getJWKS(c))
	c.Check(err, qt.IsNil)

	getKID := func() string {
		ks, err := store.GetJWKS(ctx)
		c.Check(err, qt.IsNil)

		ki := ks.Keys(ctx)
		ki.Next(ctx)
		key := ki.Pair().Value.(jwk.Key)
		_, err = uuid.Parse(key.KeyID())
		c.Check(err, qt.IsNil)
		return key.KeyID()
	}

	// Retrieve said JWKS & store it's UUID
	initialKeyId := getKID()

	// Start up the rotator, and wait a long-enough-ish time
	// for a new key to rotate
	stopRotator, err := svc.StartJWKSRotator(ctx, time.NewTicker(time.Second), time.Now())
	c.Assert(err, qt.IsNil)
	time.Sleep(time.Second)
	stopRotator()

	// Get the new rotated KID
	newKeyId := getKID()

	// And simply compare them
	c.Assert(initialKeyId, qt.Not(qt.Equals), newKeyId)
}
