// Copyright 2026 Canonical.

package rpcproxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

const testModelUUID = "00000000-0000-0000-0000-000000000001"

type testSSHKeyManager struct {
	add    func(context.Context, *openfga.User, db.SSHKeyModelFilter, sshkeys.PublicKey) error
	list   func(context.Context, *openfga.User, db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error)
	remove func(context.Context, *openfga.User, db.SSHKeyModelFilter, ...string) error
}

func (m testSSHKeyManager) AddUserPublicKey(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, key sshkeys.PublicKey) error {
	return m.add(ctx, user, model, key)
}

func (m testSSHKeyManager) ListUserPublicKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error) {
	return m.list(ctx, user, model)
}

func (m testSSHKeyManager) RemoveUserKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, targets ...string) error {
	return m.remove(ctx, user, model, targets...)
}

func newTestPublicKey(c *qt.C, comment string) sshkeys.PublicKey {
	c.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	c.Assert(err, qt.IsNil)
	publicKey, err := gossh.NewPublicKey(privateKey.Public())
	c.Assert(err, qt.IsNil)
	return sshkeys.PublicKey{PublicKey: publicKey, Comment: comment}
}

func testFacade(keyManager SSHKeyManager) keyManagerFacade {
	return keyManagerFacade{
		keyManager: keyManager,
		user:       &openfga.User{Identity: &dbmodel.Identity{Name: "alice@canonical.com"}},
		modelUUID:  testModelUUID,
	}
}

func TestKeyManagerFacadeListKeysIsModelScoped(t *testing.T) {
	c := qt.New(t)
	first := newTestPublicKey(c, "first")
	second := newTestPublicKey(c, "second")
	manager := testSSHKeyManager{
		list: func(_ context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error) {
			c.Check(user.Name, qt.Equals, "alice@canonical.com")
			c.Check(model, qt.DeepEquals, db.SSHKeyModelFilter{ModelUUID: testModelUUID})
			return []sshkeys.PublicKey{first, second}, nil
		},
	}

	facade := testFacade(manager)
	result, err := facade.ListKeys(c.Context(), jujuparams.ListSSHKeys{Mode: jujuparams.SSHListModeFingerprint})
	c.Assert(err, qt.IsNil)
	c.Check(result.Results, qt.DeepEquals, []jujuparams.StringsResult{{
		Result: []string{
			fmt.Sprintf("%s (first)", gossh.FingerprintLegacyMD5(first)),
			fmt.Sprintf("%s (second)", gossh.FingerprintLegacyMD5(second)),
		},
	}})
}

func TestKeyManagerFacadeAddKeysPersistsValidKeysOnly(t *testing.T) {
	c := qt.New(t)
	key := newTestPublicKey(c, "test-key")
	var added []sshkeys.PublicKey
	manager := testSSHKeyManager{
		add: func(_ context.Context, user *openfga.User, model db.SSHKeyModelFilter, key sshkeys.PublicKey) error {
			c.Check(user.Name, qt.Equals, "alice@canonical.com")
			c.Check(model, qt.DeepEquals, db.SSHKeyModelFilter{ModelUUID: testModelUUID})
			added = append(added, key)
			return nil
		},
	}

	facade := testFacade(manager)
	result, err := facade.AddKeys(c.Context(), jujuparams.ModifyUserSSHKeys{Keys: []string{
		"not a public key",
		marshalAuthorizedKeyWithComment(key),
	}})
	c.Assert(err, qt.IsNil)
	c.Check(added, qt.HasLen, 1)
	c.Check(added[0].Marshal(), qt.DeepEquals, key.Marshal())
	c.Check(added[0].Comment, qt.Equals, key.Comment)
	c.Check(result.Results, qt.HasLen, 1)
	c.Check(result.Results[0].Error.Message, qt.Matches, "failed to parse key.*")
}

func TestKeyManagerFacadeDeleteKeysPassesRawTargets(t *testing.T) {
	c := qt.New(t)
	key := newTestPublicKey(c, "test-key")
	fingerprint := gossh.FingerprintLegacyMD5(key)
	fullKey := marshalAuthorizedKeyWithComment(key)
	var removedTargets []string
	manager := testSSHKeyManager{
		remove: func(_ context.Context, _ *openfga.User, model db.SSHKeyModelFilter, targets ...string) error {
			c.Check(model, qt.DeepEquals, db.SSHKeyModelFilter{ModelUUID: testModelUUID})
			removedTargets = targets
			return nil
		},
	}

	facade := testFacade(manager)
	result, err := facade.DeleteKeys(c.Context(), jujuparams.ModifyUserSSHKeys{Keys: []string{"test-key", fingerprint, fullKey}})
	c.Assert(err, qt.IsNil)
	c.Check(result.Results, qt.HasLen, 0)
	c.Check(removedTargets, qt.DeepEquals, []string{"test-key", fingerprint, fullKey})
}

func TestShouldInterceptKeyManager(t *testing.T) {
	c := qt.New(t)
	for _, test := range []struct {
		version   string
		intercept bool
		err       string
	}{
		{version: "3.6.12", intercept: false},
		{version: "4.0.0", intercept: true},
		{version: "4.1.3", intercept: true},
		{version: "", err: `cannot determine KeyManager routing: controller version "" is invalid`},
		{version: "2.9.0", err: `cannot determine KeyManager routing for Juju 2.9.0`},
	} {
		c.Run(test.version, func(c *qt.C) {
			intercept, err := shouldInterceptKeyManager(test.version)
			if test.err != "" {
				c.Check(err, qt.ErrorMatches, test.err)
				return
			}
			c.Check(err, qt.IsNil)
			c.Check(intercept, qt.Equals, test.intercept)
		})
	}
}

func TestKeyManagerFacadeRoutesByControllerVersion(t *testing.T) {
	c := qt.New(t)
	key := newTestPublicKey(c, "test-key")
	request, err := json.Marshal(jujuparams.ModifyUserSSHKeys{Keys: []string{marshalAuthorizedKeyWithComment(key)}})
	c.Assert(err, qt.IsNil)

	for _, test := range []struct {
		version   string
		intercept bool
	}{
		{version: "4.0.0", intercept: true},
	} {
		c.Run(test.version, func(c *qt.C) {
			added := false
			modelProxy := modelProxy{modelUUID: testModelUUID, controllerVersion: test.version}
			if test.intercept {
				modelProxy.sshKeyManager = testSSHKeyManager{
					add: func(_ context.Context, _ *openfga.User, model db.SSHKeyModelFilter, addedKey sshkeys.PublicKey) error {
						c.Check(model, qt.DeepEquals, db.SSHKeyModelFilter{ModelUUID: testModelUUID})
						c.Check(addedKey.Marshal(), qt.DeepEquals, key.Marshal())
						added = true
						return nil
					},
				}
			}
			proxy := clientProxy{
				modelProxy: modelProxy,
				user:       &openfga.User{Identity: &dbmodel.Identity{Name: "alice@canonical.com"}},
			}
			message := &message{Type: "KeyManager", Request: "AddKeys", Params: request}
			clientResponse, err := proxy.handleKeyManagerFacade(c.Context(), message)
			c.Assert(err, qt.IsNil)
			c.Check(added, qt.IsTrue)
			c.Check(clientResponse, qt.Equals, message)
		})
	}
}
