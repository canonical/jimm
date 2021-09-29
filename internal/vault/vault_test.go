// Copyright 2021 Canonical Ltd.

package vault_test

import (
	"context"
	"log"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/vault"
)

func TestMain(m *testing.M) {
	err := jimmtest.StartVault()
	if err != nil {
		log.Printf("error starting vault: %s\n", err)
	}
	code := m.Run()
	jimmtest.StopVault()
	os.Exit(code)
}

func newStore(t testing.TB) *vault.VaultCloudCredentialAttributeStore {
	client, path, creds, ok := jimmtest.VaultClient(t)
	if !ok {
		t.Skip("vault not available")
	}
	return &vault.VaultCloudCredentialAttributeStore{
		Client:     client,
		AuthSecret: creds,
		AuthPath:   jimmtest.VaultAuthPath,
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
