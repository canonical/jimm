// Copyright 2021 Canonical Ltd.

package vault

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

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
	resetJWKS(c, store)

	// So, we first put a fresh JWKS in the store
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

func TestGetJWKSPrivateKey(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	resetJWKS(c, store)

	store.PutJWKS(ctx, time.Now().AddDate(0, 3, 1))
	keyPem, err := store.GetJWKSPrivateKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(string(keyPem), qt.Contains, "-----BEGIN RSA PRIVATE KEY-----")
}

// This can be considered an e2e test for the JWKS EP and our current credential storage.
// It verifies signatures do work as intended.
//
// This is also how I would imagine a juju controller would run through the process
// of verification without the use of x5t & x5c.
//
// As we often just forget these processes I've left neatly organised /**/ comments
// denoting each segment of the process.
func TestSigningAJWT(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	resetJWKS(c, store)
	err := store.PutJWKS(ctx, time.Now().UTC().AddDate(0, 3, 1))
	c.Check(err, qt.IsNil)
	jwtId := "1234-1234-1234-1234"

	/*
		Server gets public only JWKS from store
	*/
	set, err := store.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)

	/*
		Server gets JWKS Public key ID from retrieved JWKS
	*/
	key, ok := set.Key(0) // Fine for test, we know there's only one.
	c.Assert(ok, qt.IsTrue)
	jwksKid := key.KeyID()
	c.Check(jwksKid, qt.HasLen, 36) // Our UUIDs are always 36len

	/*
		Server gets private key for said public JWKS from store

		Our keys are persisted in PEM (passphraseless but we could add a passphrase?) B64 for consistency across the wire.

		TODO@ales@kian: Shall we use passphrases on the current keysets private key, is it worth it?
	*/
	privKeyPem, err := store.GetJWKSPrivateKey(ctx)
	c.Check(err, qt.IsNil)

	/*
		Server decodes the PEM encoded private key
	*/
	block, _ := pem.Decode([]byte(privKeyPem))
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	c.Check(err, qt.IsNil)

	/*
		Server sets up a signing key from the decoded PEM private key
		and manually sets the algorithm and keyid
	*/
	signingKey, err := jwk.FromRaw(privKey)
	signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
	signingKey.Set(jwk.KeyIDKey, jwksKid)
	c.Assert(err, qt.IsNil)

	/*
		Server sets up a JWT
	*/
	token, err := jwt.NewBuilder().
		Issuer("jimmy").
		Audience([]string{"controller-somecontroller"}).
		Subject("user-alice@external").
		Issuer("jimm").
		JwtID(jwtId).
		Claim("access", map[string]interface{}{ // It is important to send it as 'interface', as private claims can be string, num or bool
			"controller-1234-1234-1234": "administrator",
			"model-1234-1234-1234":      "administrator",
		}).
		Build()
	c.Check(err, qt.IsNil)

	/*
		Server now signs the JWT with the prepared signing key, based on the current active JWKS
	*/
	freshJwt, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, signingKey))
	c.Check(err, qt.IsNil)
	/*
		Server makes request to controller with the JWT
	*/

	/*
		Juju controller:
		1. Recieves request (hypothetically)
		2. Checks it's jws.Cache for a JWKS (and if not present, gets a fresh set) (hypothetically)
		3. Verifies JWT using the JWKS
		4. Goes on to do what it does...
	*/
	verifiedToken, err := jwt.Parse(
		freshJwt,
		jwt.WithKeySet(set),
	)
	c.Assert(err, qt.IsNil)
	// The token irritatingly has no exported fields. And as cmp cannot handle unexported,
	// we test one, by, one...
	c.Assert(verifiedToken.JwtID(), qt.Equals, token.JwtID())
	c.Assert(verifiedToken.Issuer(), qt.Equals, token.Issuer())
	c.Assert(verifiedToken.Audience(), qt.DeepEquals, token.Audience())
	c.Assert(verifiedToken.Subject(), qt.Equals, token.Subject())
	c.Assert(verifiedToken.PrivateClaims(), qt.DeepEquals, token.PrivateClaims())

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
