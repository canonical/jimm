// Copyright 2025 Canonical.

package sshkeys_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	gossh "golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type sshKeysManagerSuite struct {
	manager    *sshkeys.SSHKeyManager
	user       *openfga.User
	db         *db.Database
	ofgaClient *openfga.OFGAClient
	pubKey     sshkeys.PublicKey
}

func (s *sshKeysManagerSuite) Init(c *qt.C) {
	// Setup DB
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.manager, err = sshkeys.NewSSHKeyManager(db)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(i).Error, qt.IsNil)
	s.user = openfga.NewUser(i, ofgaClient)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)

	pubKey, err := gossh.NewPublicKey(&key.PublicKey)
	c.Assert(err, qt.IsNil)
	s.pubKey = sshkeys.PublicKey{PublicKey: pubKey, Comment: "myComment"}
}

func (s *sshKeysManagerSuite) TestAddUserPublicKey(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	err := s.manager.AddUserPublicKey(ctx, s.user, s.pubKey)
	c.Assert(err, qt.IsNil)

	var dbKey dbmodel.SSHKey
	c.Assert(s.db.DB.First(&dbKey).Error, qt.IsNil)
	c.Assert(dbKey.ID, qt.Not(qt.Equals), 0)
	c.Assert(dbKey.IdentityName, qt.Equals, "alice")
	c.Assert(dbKey.PublicKey, qt.DeepEquals, s.pubKey.Marshal())
	c.Assert(dbKey.MD5Fingerprint, qt.Equals, gossh.FingerprintLegacyMD5(s.pubKey))
	c.Assert(dbKey.KeyComment, qt.Equals, s.pubKey.Comment)
}

func (s *sshKeysManagerSuite) TestListUserPublicKeys(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	err := s.manager.AddUserPublicKey(ctx, s.user, s.pubKey)
	c.Assert(err, qt.IsNil)

	keys, err := s.manager.ListUserPublicKeys(ctx, s.user)
	c.Assert(err, qt.IsNil)

	c.Assert(keys, qt.HasLen, 1)
	c.Assert(keys[0].Comment, qt.Equals, s.pubKey.Comment)
	c.Assert(keys[0].Marshal(), qt.DeepEquals, s.pubKey.Marshal())
}

func (s *sshKeysManagerSuite) TestRemoveUserKeyByComment(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	err := s.manager.AddUserPublicKey(ctx, s.user, s.pubKey)
	c.Assert(err, qt.IsNil)

	var key dbmodel.SSHKey
	c.Assert(s.db.DB.First(&dbmodel.SSHKey{}).First(&key).Error, qt.IsNil)

	err = s.manager.RemoveUserKeyByComment(ctx, s.user, s.pubKey.Comment)
	c.Assert(err, qt.IsNil)

	c.Assert(s.db.DB.First(&dbmodel.SSHKey{}).Error, qt.Equals, gorm.ErrRecordNotFound)
}

func (s *sshKeysManagerSuite) TestRemoveUserKeyByFingerprint(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	err := s.manager.AddUserPublicKey(ctx, s.user, s.pubKey)
	c.Assert(err, qt.IsNil)

	var key dbmodel.SSHKey
	c.Assert(s.db.DB.First(&dbmodel.SSHKey{}).First(&key).Error, qt.IsNil)

	err = s.manager.RemoveUserKeyByFingerprint(ctx, s.user, gossh.FingerprintLegacyMD5(s.pubKey))
	c.Assert(err, qt.IsNil)

	c.Assert(s.db.DB.First(&dbmodel.SSHKey{}).Error, qt.Equals, gorm.ErrRecordNotFound)
}

func TestSSHKeyManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshKeysManagerSuite{})
}
