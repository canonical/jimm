// Copyright 2025 Canonical.

package vault_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/vault"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func newStore(t testing.TB) *vault.VaultStore {
	client, path, roleID, secretID, ok := jimmtest.VaultClient(t)
	if !ok {
		t.Skip("vault not available")
	}
	return &vault.VaultStore{
		Client:       client,
		RoleID:       roleID,
		RoleSecretID: secretID,
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

func TestVaultCloudCredentialAttributeStoreRoundTrip(t *testing.T) {
	c := qt.New(t)

	st := newStore(c)
	ctx := context.Background()
	tag := names.NewCloudCredentialTag("aws/alice@canonical.com/" + c.Name())
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
	tag := names.NewCloudCredentialTag("aws/alice@canonical.com/" + c.Name())

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

func TestGetAndPutJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	err = store.PutJWKS(ctx, getJWKS(c))
	c.Assert(err, qt.IsNil)
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

func TestGetAndPutJWKSExpiry(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	expectedExpiry := time.Now().AddDate(0, 3, 1)
	err = store.PutJWKSExpiry(ctx, expectedExpiry)
	c.Assert(err, qt.IsNil)
	expiry, err := store.GetJWKSExpiry(ctx)
	c.Assert(err, qt.IsNil)
	// We really care just for the month, not exact Us, but we use RFC3339
	// everywhere, so it made sense to just use it here.
	c.Assert(expiry.Month(), qt.Equals, expectedExpiry.Month())
}

func TestGetAndPutJWKSPrivateKey(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	c.Assert(err, qt.IsNil)
	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(keySet),
		},
	)

	err = store.PutJWKSPrivateKey(ctx, privateKeyPEM)
	c.Assert(err, qt.IsNil)
	err = store.PutJWKS(ctx, getJWKS(c))
	c.Assert(err, qt.IsNil)
	keyPem, err := store.GetJWKSPrivateKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(string(keyPem), qt.Contains, "-----BEGIN RSA PRIVATE KEY-----")
}
func TestGetKeyMetadataEmpty(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)

	metadata, err := store.GetKeyMetadata(ctx)
	c.Assert(err, qt.IsNil)
	// Should return zero value when not set
	c.Assert(metadata.Keys, qt.HasLen, 0)
	c.Assert(metadata.CurrentActiveKID, qt.Equals, "")
}

func TestPutAndGetKeyMetadata(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)

	now := time.Now()
	activeAt := now.Add(-1 * time.Hour)
	preActiveAt := now.Add(30 * time.Minute)
	oldAt := now.Add(-100 * time.Hour)

	metadata := vault.KeyMetadata{
		Keys: []vault.KeyInfo{
			{
				KID:         "key-1",
				PrivateKey:  "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----",
				PublicJWK:   `{"kty":"RSA","kid":"key-1"}`,
				ActivatedAt: &activeAt,
			},
			{
				KID:         "key-2",
				PrivateKey:  "",
				PublicJWK:   `{"kty":"RSA","kid":"key-2"}`,
				ActivatedAt: &preActiveAt,
			},
			{
				KID:         "key-0",
				PrivateKey:  "",
				PublicJWK:   `{"kty":"RSA","kid":"key-0"}`,
				ActivatedAt: &oldAt,
			},
		},
		CurrentActiveKID: "key-1",
		MaxTokenLifetime: 1 * time.Hour,
		GracePeriod:      15 * time.Minute,
		RotationInterval: 2160 * time.Hour, // 90 days
	}

	err := store.PutKeyMetadata(ctx, metadata)
	c.Assert(err, qt.IsNil)

	retrieved, err := store.GetKeyMetadata(ctx)
	c.Assert(err, qt.IsNil)

	c.Assert(retrieved.CurrentActiveKID, qt.Equals, "key-1")
	c.Assert(retrieved.Keys, qt.HasLen, 3)
	c.Assert(retrieved.Keys[0].KID, qt.Equals, "key-1")
	c.Assert(retrieved.Keys[0].ActivatedAt, qt.IsNotNil)
	c.Assert(retrieved.Keys[1].KID, qt.Equals, "key-2")
	c.Assert(retrieved.Keys[2].KID, qt.Equals, "key-0")
	c.Assert(retrieved.Keys[2].ActivatedAt, qt.IsNotNil)
	c.Assert(retrieved.MaxTokenLifetime, qt.Equals, 1*time.Hour)
	c.Assert(retrieved.GracePeriod, qt.Equals, 15*time.Minute)
	c.Assert(retrieved.RotationInterval, qt.Equals, 2160*time.Hour)
}

func TestUpdateKeyMetadata(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := newStore(c)

	now := time.Now()
	activeAt := now
	preActiveAt := now.Add(1 * time.Hour)

	initial := vault.KeyMetadata{
		Keys: []vault.KeyInfo{
			{
				KID:         "key-1",
				PrivateKey:  "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----",
				PublicJWK:   `{"kty":"RSA","kid":"key-1"}`,
				ActivatedAt: &activeAt,
			},
		},
		CurrentActiveKID: "key-1",
		MaxTokenLifetime: 1 * time.Hour,
		GracePeriod:      15 * time.Minute,
		RotationInterval: 2160 * time.Hour,
	}

	err := store.PutKeyMetadata(ctx, initial)
	c.Assert(err, qt.IsNil)

	// Retrieve and modify.
	retrieved, err := store.GetKeyMetadata(ctx)
	c.Assert(err, qt.IsNil)

	// Add newww key as preactive (ActivatedAt is in the future).
	retrieved.Keys = append(retrieved.Keys, vault.KeyInfo{
		KID:         "key-2",
		PrivateKey:  "",
		PublicJWK:   `{"kty":"RSA","kid":"key-2"}`,
		ActivatedAt: &preActiveAt,
	})

	err = store.PutKeyMetadata(ctx, retrieved)
	c.Assert(err, qt.IsNil)

	// Verify changes persisted
	final, err := store.GetKeyMetadata(ctx)
	c.Assert(err, qt.IsNil)

	c.Assert(final.Keys, qt.HasLen, 2)
	c.Assert(final.Keys[0].KID, qt.Equals, "key-1")
	c.Assert(final.Keys[1].KID, qt.Equals, "key-2")
	c.Assert(final.Keys[1].ActivatedAt, qt.IsNotNil)
	// key-2's ActivatedAt is in the future, so it's pre-active
}
