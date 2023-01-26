// Copyright 2021 Canonical Ltd.

package vault

import (
	"context"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func newStore(t testing.TB) *VaultStore {
	client, path, creds, ok := jimmtest.VaultClient(t, "../../")
	if !ok {
		t.Skip("vault not available")
	}
	return &VaultStore{
		Client:     client,
		AuthSecret: creds,
		AuthPath:   "/auth/approle/login",
		KVPath:     path,
	}
}

func TestVaultCloudCredentialAttributeStoreRoundTrip(t *testing.T) {
	c := qt.New(t)

	st := newStore(c)
	ctx := context.Background()
	tag := names.NewCloudCredentialTag("aws/alice@external/" + c.Name())
	err := st.Put(ctx, tag, map[string]string{"a": "A", "b": "1234"})
	c.Assert(err, qt.IsNil)

	attr, err := st.Get(ctx, tag)
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.DeepEquals, map[string]string{"a": "A", "b": "1234"})

	err = st.Put(ctx, tag, nil)
	c.Assert(err, qt.IsNil)
	attr, err = st.Get(ctx, tag)
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.HasLen, 0)
}

func TestVaultCloudCredentialAtrributeStoreEmpty(t *testing.T) {
	c := qt.New(t)

	st := newStore(c)
	ctx := context.Background()
	tag := names.NewCloudCredentialTag("aws/alice@external/" + c.Name())

	attr, err := st.Get(ctx, tag)
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.HasLen, 0)

	err = st.Put(ctx, tag, attr)
	c.Assert(err, qt.IsNil)

	attr, err = st.Get(ctx, tag)
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.HasLen, 0)
}

func TestVaultControllerCredentialsStoreRoundTrip(t *testing.T) {
	c := qt.New(t)

	st := newStore(c)
	ctx := context.Background()
	controllerName := "controller-1"
	username := "user1"
	password := "secret-password"
	err := st.PutControllerCredentials(ctx, controllerName, username, password)
	c.Assert(err, qt.IsNil)

	u, p, err := st.GetControllerCredentials(ctx, controllerName)
	c.Assert(err, qt.IsNil)
	c.Check(u, qt.Equals, username)
	c.Check(p, qt.Equals, password)

	err = st.PutControllerCredentials(ctx, controllerName, "", "")
	c.Assert(err, qt.IsNil)
	u, p, err = st.GetControllerCredentials(ctx, controllerName)
	c.Assert(err, qt.IsNil)
	c.Check(u, qt.Equals, "")
	c.Check(p, qt.Equals, "")
}

func TestGetJWKSPath(t *testing.T) {
	c := qt.New(t)

	store := newStore(c)
	c.Assert(store.getJWKSPath(), qt.Equals, store.KVPath+"creds/.well-known/jwks.json")
}

func TestGenerateJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	store := newStore(c)
	jwk, privKeyPem, err := store.generateJWK(ctx)
	c.Assert(err, qt.IsNil)

	// kid
	_, err = uuid.Parse(jwk.KeyID())
	c.Assert(err, qt.IsNil)
	// use
	c.Assert(jwk.KeyUsage(), qt.Equals, "sig")
	// alg
	c.Assert(jwk.Algorithm(), qt.Equals, jwa.RS256)

	// It's fine for us to just test the key exists.
	c.Assert(string(privKeyPem), qt.Contains, "-----BEGIN RSA PRIVATE KEY-----")
}

func TestPutJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	store := newStore(c)
	resetJWKS(c, store)

	err := store.PutJWKS(ctx, time.Now().AddDate(0, 3, 1))
	c.Assert(err, qt.IsNil)
}

func TestGetJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	store := newStore(c)
	resetJWKS(c, store)

	store.PutJWKS(ctx, time.Now().AddDate(0, 3, 1))

	ks, err := store.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	ki := ks.Keys(ctx)
	ki.Next(ctx)
	key := ki.Pair().Value.(jwk.Key)

	_, err = uuid.Parse(key.KeyID())
	c.Assert(err, qt.IsNil)

	c.Assert(key.KeyUsage(), qt.Equals, "sig")
	c.Assert(key.Algorithm(), qt.Equals, jwa.RS256)
}

func TestGetJWKSExpiry(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	resetJWKS(c, store)

	store.PutJWKS(ctx, time.Now().AddDate(0, 3, 1))
	expiry, err := store.getJWKSExpiry(ctx)
	c.Assert(err, qt.IsNil)
	// We really care just for the month, not exact Us, but we use RFC3339
	// everywhere, so it made sense to just use it here.
	c.Assert(expiry.Month(), qt.Equals, time.Now().AddDate(0, 3, 1).Month())
}

// This test is difficult to gauge, as it is truly only time based.
// As such we take a -3 deficit to our total suites test time.
// But this is a fair usecase for time-based-testing.
func TestStartJWKSRotatorWithNoJWKSInTheStore(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	resetJWKS(c, store)

	cron, id, err := store.StartJWKSRotator(ctx, "@every 1s", time.Now())
	c.Assert(cron.Entry(id).Valid(), qt.IsTrue)
	c.Assert(err, qt.IsNil)
	time.Sleep(time.Second * 3)
	defer cron.Stop()

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

	// So, we first put a fresh JWKS in the store
	resetJWKS(c, store)

	err := store.PutJWKS(ctx, time.Now())
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
	cron, id, err := store.StartJWKSRotator(ctx, "@every 1s", time.Now())
	c.Check(cron.Entry(id).Valid(), qt.IsTrue)
	c.Check(err, qt.IsNil)
	time.Sleep(time.Second * 3)
	defer cron.Stop()

	// Get the new rotated KID
	newKeyId := getKID()

	// And simply compare them
	c.Assert(initialKeyId, qt.Not(qt.Equals), newKeyId)
}

func resetJWKS(c *qt.C, store *VaultStore) {
	vc, err := store.client(context.Background())
	c.Check(err, qt.IsNil)

	_, err = vc.Logical().Delete(store.getJWKSExpiryPath())
	c.Check(err, qt.IsNil)

	_, err = vc.Logical().Delete(store.getJWKSPath())
	c.Check(err, qt.IsNil)

	_, err = vc.Logical().Delete(store.getJWKSPrivateKeyPath())
	c.Check(err, qt.IsNil)
}
