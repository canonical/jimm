// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"

	qt "github.com/frankban/quicktest"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func (s *dbSuite) TestCreateSSHKey(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u).Error, qt.IsNil)

	key := dbmodel.SSHKey{
		Identity:       *u,
		PublicKey:      []byte("foo"),
		KeyComment:     "bar",
		MD5Fingerprint: "fake-fingerprint",
	}
	err = s.Database.AddSSHKey(context.Background(), &key)
	c.Assert(err, qt.IsNil)

	var gotKey dbmodel.SSHKey
	c.Assert(s.Database.DB.First(&gotKey).Error, qt.IsNil)
	c.Assert(gotKey.ID, qt.Not(qt.Equals), 0)
	c.Assert(gotKey.IdentityName, qt.Equals, "bob@canonical.com")
	c.Assert(string(gotKey.PublicKey), qt.Equals, "foo")
	c.Assert(gotKey.KeyComment, qt.Equals, "bar")

	err = s.Database.AddSSHKey(context.Background(), &key)
	c.Assert(err, qt.ErrorMatches, `.*duplicate key value violates unique constraint.*`)
}

func (s *dbSuite) TestListSSHKeysForUser(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u).Error, qt.IsNil)

	u2, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u2).Error, qt.IsNil)

	key := dbmodel.SSHKey{Identity: *u, PublicKey: []byte("foo"), KeyComment: "bar", MD5Fingerprint: "fake-fingerprint"}
	c.Assert(s.Database.DB.Create(&key).Error, qt.IsNil)

	key2 := dbmodel.SSHKey{Identity: *u, PublicKey: []byte("foo2"), KeyComment: "bar2", MD5Fingerprint: "fake-fingerprint"}
	c.Assert(s.Database.DB.Create(&key2).Error, qt.IsNil)

	// Key3 is owned by Alice and should not be returned.
	key3 := dbmodel.SSHKey{Identity: *u2, PublicKey: []byte("foo3"), KeyComment: "bar3", MD5Fingerprint: "fake-fingerprint"}
	c.Assert(s.Database.DB.Create(&key3).Error, qt.IsNil)

	gotKeys, err := s.Database.ListSSHKeysForUser(context.Background(), "bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(gotKeys, qt.HasLen, 2)
	c.Assert(gotKeys[0].IdentityName, qt.Equals, "bob@canonical.com")
	c.Assert(gotKeys[0].KeyComment, qt.Equals, "bar")
	c.Assert(gotKeys[0].MD5Fingerprint, qt.Equals, "fake-fingerprint")
	c.Assert(string(gotKeys[0].PublicKey), qt.Equals, "foo")
	c.Assert(gotKeys[1].IdentityName, qt.Equals, "bob@canonical.com")
	c.Assert(gotKeys[1].KeyComment, qt.Equals, "bar2")
	c.Assert(gotKeys[1].MD5Fingerprint, qt.Equals, "fake-fingerprint")
	c.Assert(string(gotKeys[1].PublicKey), qt.Equals, "foo2")
}

func (s *dbSuite) TestRemoveSSHKeyByFingerprint(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u).Error, qt.IsNil)

	u2, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u2).Error, qt.IsNil)

	//nolint:gosec // Don't need secure bits for test.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 512)
	c.Assert(err, qt.IsNil)
	publicKey, err := gossh.NewPublicKey(&rsaKey.PublicKey)
	c.Assert(err, qt.IsNil)

	key := dbmodel.SSHKey{Identity: *u, PublicKey: publicKey.Marshal(), KeyComment: "bar", MD5Fingerprint: gossh.FingerprintLegacyMD5(publicKey)}
	c.Assert(s.Database.DB.Create(&key).Error, qt.IsNil)

	// key2 with the same public-key but different owner should not be deleted.
	key2 := dbmodel.SSHKey{Identity: *u2, PublicKey: publicKey.Marshal(), KeyComment: "bar", MD5Fingerprint: gossh.FingerprintLegacyMD5(publicKey)}
	c.Assert(s.Database.DB.Create(&key2).Error, qt.IsNil)

	var keyCount int64
	c.Assert(s.Database.DB.Model(&dbmodel.SSHKey{}).Count(&keyCount).Error, qt.IsNil)
	c.Assert(keyCount, qt.Equals, int64(2))

	err = s.Database.RemoveSSHKeyByFingerprint(context.Background(), "bob@canonical.com", gossh.FingerprintLegacyMD5(publicKey))
	c.Assert(err, qt.IsNil)

	var keys []dbmodel.SSHKey
	c.Assert(s.Database.DB.Find(&keys).Error, qt.IsNil)
	c.Assert(keys, qt.HasLen, 1)
	c.Assert(keys[0].IdentityName, qt.Equals, "alice@canonical.com")
	c.Assert(keys[0].PublicKey, qt.DeepEquals, publicKey.Marshal())
}

func (s *dbSuite) TestRemoveSSHKeyByComment(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u).Error, qt.IsNil)

	// key is expected to be removed.
	key := dbmodel.SSHKey{Identity: *u, PublicKey: []byte("foo"), KeyComment: "aaa"}
	c.Assert(s.Database.DB.Create(&key).Error, qt.IsNil)

	// key2 belongs to the same user as key but is not expected to be removed.
	key2 := dbmodel.SSHKey{Identity: *u, PublicKey: []byte("hello"), KeyComment: "bbb"}
	c.Assert(s.Database.DB.Create(&key2).Error, qt.IsNil)

	u2, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u2).Error, qt.IsNil)

	// key3 belongs to a different user and has the same comment as key.
	key3 := dbmodel.SSHKey{Identity: *u2, PublicKey: []byte("foo"), KeyComment: "ccc"}
	c.Assert(s.Database.DB.Create(&key3).Error, qt.IsNil)

	var keyCount int64
	c.Assert(s.Database.DB.Model(&dbmodel.SSHKey{}).Count(&keyCount).Error, qt.IsNil)
	c.Assert(keyCount, qt.Equals, int64(3))

	err = s.Database.RemoveSSHKeyByComment(context.Background(), "bob@canonical.com", "aaa")
	c.Assert(err, qt.IsNil)

	var keys []dbmodel.SSHKey
	c.Assert(s.Database.DB.Order("key_comment DESC").Find(&keys).Error, qt.IsNil)
	c.Assert(keys, qt.HasLen, 2)
	c.Assert(keys[0].KeyComment, qt.Equals, "ccc")
	c.Assert(keys[0].IdentityName, qt.Equals, "alice@canonical.com")
	c.Assert(keys[1].KeyComment, qt.Equals, "bbb")
	c.Assert(keys[1].IdentityName, qt.Equals, "bob@canonical.com")

	// Removal with no key should return an error.
	err = s.Database.RemoveSSHKeyByComment(context.Background(), "bob@canonical.com", "fake-key")
	c.Assert(err, qt.ErrorMatches, "key not found")
}
