// Copyright 2021 Canonical Ltd.

package vault

import (
	"context"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v4"

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

func TestGenerateJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	store := newStore(c)
	jwk, err := store.generateJWKS(ctx)
	c.Assert(err, qt.IsNil)

	// kid
	_, err = uuid.Parse(jwk.KeyID())
	c.Assert(err, qt.IsNil)
	// use
	c.Assert(jwk.KeyUsage(), qt.Equals, "sig")
	// alg
	c.Assert(jwk.Algorithm().String(), qt.Equals, "RS256")
}

func TestPutJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	store := newStore(c)

	err := store.PutJWKS(ctx)
	c.Assert(err, qt.IsNil)
}
