// Copyright 2025 Canonical.

package rpcproxy_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"regexp"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/utils/v3/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpcproxy"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type keyManagerFacadeSuite struct {
	keyManagerFacade        rpcproxy.KeyManagerFacade
	key1                    sshkeys.PublicKey
	key2                    sshkeys.PublicKey
	addKeyF                 func(ctx context.Context, user *openfga.User, publicKey sshkeys.PublicKey) error
	removeKeyByCommentF     func(ctx context.Context, user *openfga.User, comment string) error
	removeKeyByFingerprintF func(ctx context.Context, user *openfga.User, fingerprint string) error
}

func (k *keyManagerFacadeSuite) Init(c *qt.C) {
	pk1, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)
	pk2, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)

	pubKey1, err := gossh.NewPublicKey(&pk1.PublicKey)
	c.Assert(err, qt.IsNil)

	pubKey2, err := gossh.NewPublicKey(&pk2.PublicKey)
	c.Assert(err, qt.IsNil)

	k.key1 = sshkeys.PublicKey{
		PublicKey: pubKey1,
		Comment:   "comment-1",
	}

	k.key2 = sshkeys.PublicKey{
		PublicKey: pubKey2,
		Comment:   "comment-2",
	}

	keyManager := mocks.SSHKeyManager{
		ListUserPublicKeys_: func(ctx context.Context, user *openfga.User) ([]sshkeys.PublicKey, error) {
			return []sshkeys.PublicKey{k.key1, k.key2}, nil
		},
		AddUserPublicKey_: func(ctx context.Context, user *openfga.User, publicKey sshkeys.PublicKey) error {
			return k.addKeyF(ctx, user, publicKey)
		},
		RemoveUserKeyByComment_: func(ctx context.Context, user *openfga.User, comment string) error {
			return k.removeKeyByCommentF(ctx, user, comment)
		},
		RemoveUserKeyByFingerprint_: func(ctx context.Context, user *openfga.User, fingerprint string) error {
			return k.removeKeyByFingerprintF(ctx, user, fingerprint)
		},
	}
	k.keyManagerFacade = rpcproxy.NewKeyManagerFacade(&keyManager, nil)
}

var isFingerprintRegex = regexp.MustCompile(`[0-9a-f]{2}(:[0-9a-f]{2}){15}`)

func (k *keyManagerFacadeSuite) TestListKeysShort(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	res, err := k.keyManagerFacade.ListKeys(ctx, params.ListSSHKeys{Mode: ssh.Fingerprints})
	c.Assert(err, qt.IsNil)

	c.Assert(res.Results[0].Result, qt.HasLen, 2)
	c.Assert(res.Results[0].Result[0], qt.Matches, `.+ \(comment-1\)`)
	c.Assert(isFingerprintRegex.MatchString(res.Results[0].Result[0]), qt.IsTrue)
	c.Assert(res.Results[0].Result[1], qt.Matches, `.+ \(comment-2\)`)
	c.Assert(isFingerprintRegex.MatchString(res.Results[0].Result[1]), qt.IsTrue)
}

func (k *keyManagerFacadeSuite) TestListKeysFull(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	res, err := k.keyManagerFacade.ListKeys(ctx, params.ListSSHKeys{Mode: ssh.FullKeys})
	c.Assert(err, qt.IsNil)

	c.Assert(res.Results[0].Result, qt.HasLen, 2)
	c.Assert(res.Results[0].Result[0], qt.Matches, `ssh-rsa .+ comment-1\n`)
	c.Assert(res.Results[0].Result[1], qt.Matches, `ssh-rsa .+ comment-2\n`)
}

func (k *keyManagerFacadeSuite) TestAddKeys(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	k.addKeyF = func(ctx context.Context, user *openfga.User, publicKey sshkeys.PublicKey) error {
		c.Check(publicKey.Marshal(), qt.DeepEquals, k.key1.Marshal())
		c.Check(publicKey.Comment, qt.Equals, k.key1.Comment)
		return nil
	}
	var b bytes.Buffer
	e := base64.NewEncoder(base64.StdEncoding, &b)
	_, _ = e.Write(k.key1.Marshal()) // Writes to a bytes buffer always return a nil error.
	e.Close()
	authorizedKey := fmt.Sprintf("%s %s %s", k.key1.Type(), &b, k.key1.Comment)
	_, _ = k.keyManagerFacade.AddKeys(ctx, params.ModifyUserSSHKeys{Keys: []string{authorizedKey}})
}

func (k *keyManagerFacadeSuite) TestDeleteKeysByComment(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	k.removeKeyByCommentF = func(ctx context.Context, user *openfga.User, comment string) error {
		c.Check(comment, qt.Equals, "comment-1")
		return nil
	}
	_, _ = k.keyManagerFacade.DeleteKeys(ctx, params.ModifyUserSSHKeys{Keys: []string{"comment-1"}})
}

func (k *keyManagerFacadeSuite) TestDeleteKeysByFingerprint(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	fp := gossh.FingerprintLegacyMD5(k.key1)

	k.removeKeyByFingerprintF = func(ctx context.Context, user *openfga.User, fingerprint string) error {
		c.Check(fingerprint, qt.Equals, fp)
		return nil
	}
	_, _ = k.keyManagerFacade.DeleteKeys(ctx, params.ModifyUserSSHKeys{Keys: []string{fp}})
}

func TestKeyManagerFacade(t *testing.T) {
	qtsuite.Run(qt.New(t), &keyManagerFacadeSuite{})
}
