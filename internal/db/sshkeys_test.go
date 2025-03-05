// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"
	"github.com/juju/juju/state"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type sshKeysSuite struct {
	Database *db.Database
	User     dbmodel.Identity
	Model    dbmodel.Model
	Model2   dbmodel.Model
}

func (s *sshKeysSuite) Init(c *qt.C) {
	ctx := context.Background()
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	s.Database = db
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	err = s.Database.Migrate(ctx)
	c.Assert(err, qt.Equals, nil)
	user, _, controller, model, _, cloud, cloudCred, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.Database)
	id, _ := uuid.NewRandom()
	model2 := dbmodel.Model{
		Name: petname.Generate(2, "-"),
		UUID: sql.NullString{
			String: id.String(),
			Valid:  true,
		},
		OwnerIdentityName: user.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cloudCred.ID,
		Life:              state.Alive.String(),
	}
	err = s.Database.AddModel(ctx, &model2)
	c.Assert(err, qt.IsNil)
	s.User = user
	s.Model = model
	s.Model2 = model2
}

func (s *sshKeysSuite) TestCreateSSHKey(c *qt.C) {
	ctx := context.Background()
	key := dbmodel.SSHKey{
		Identity:       s.User,
		PublicKey:      []byte("foo"),
		KeyComment:     "bar",
		MD5Fingerprint: "fake-fingerprint",
		Model:          s.Model,
	}
	err := s.Database.AddSSHKey(ctx, &key)
	c.Assert(err, qt.IsNil)

	keys, err := s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 1)
	c.Assert(keys[0].IdentityName, qt.Equals, s.User.Name)
	c.Assert(keys[0].ModelUUID, qt.Equals, s.Model.UUID.String)

	// add the same key twice, expect no error
	err = s.Database.AddSSHKey(ctx, &key)
	c.Assert(err, qt.IsNil)

	// add the same key to another model, expect no error
	key.Model = s.Model2
	err = s.Database.AddSSHKey(ctx, &key)
	c.Assert(err, qt.IsNil)
}

func (s *sshKeysSuite) TestRemoveSSHKeyByFingerprint(c *qt.C) {
	ctx := context.Background()

	u2, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u2).Error, qt.IsNil)

	//nolint:gosec // Don't need secure bits for test.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 512)
	c.Assert(err, qt.IsNil)
	publicKey, err := gossh.NewPublicKey(&rsaKey.PublicKey)
	c.Assert(err, qt.IsNil)

	key := dbmodel.SSHKey{
		Identity:       s.User,
		PublicKey:      publicKey.Marshal(),
		KeyComment:     "bar",
		MD5Fingerprint: gossh.FingerprintLegacyMD5(publicKey),
		Model:          s.Model,
	}
	c.Assert(s.Database.AddSSHKey(ctx, &key), qt.IsNil)

	// key2 with the same public-key but different owner should not be deleted.
	key2 := dbmodel.SSHKey{
		Identity:       *u2,
		PublicKey:      publicKey.Marshal(),
		KeyComment:     "bar",
		MD5Fingerprint: gossh.FingerprintLegacyMD5(publicKey),
		Model:          s.Model,
	}
	c.Assert(s.Database.AddSSHKey(ctx, &key2), qt.IsNil)

	err = s.Database.RemoveSSHKeyByFingerprint(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String}, gossh.FingerprintLegacyMD5(publicKey))
	c.Assert(err, qt.IsNil)

	keys, err := s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 0)

	keys, err = s.Database.ListSSHKeysForUser(ctx, u2.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 1)
}

func (s *sshKeysSuite) TestListSSHKeysForUser(c *qt.C) {
	ctx := context.Background()

	key := dbmodel.SSHKey{
		Identity:       s.User,
		PublicKey:      []byte("foo"),
		KeyComment:     "bar",
		MD5Fingerprint: "fake-fingerprint",
		Model:          s.Model,
	}
	err := s.Database.AddSSHKey(ctx, &key)
	c.Assert(err, qt.IsNil)

	keys, err := s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 1)
	c.Assert(keys[0].IdentityName, qt.Equals, s.User.Name)
	c.Assert(keys[0].ModelUUID, qt.Equals, s.Model.UUID.String)
	key2 := dbmodel.SSHKey{
		Identity:       s.User,
		PublicKey:      []byte("foo1"),
		KeyComment:     "bar",
		MD5Fingerprint: "fake-fingerprint",
		Model:          s.Model2,
	}
	c.Assert(s.Database.AddSSHKey(ctx, &key2), qt.IsNil)
	keys, err = s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model2.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 1)
	c.Assert(keys[0].IdentityName, qt.Equals, s.User.Name)
	c.Assert(keys[0].ModelUUID, qt.Equals, s.Model2.UUID.String)

	keys, err = s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{All: true})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 2)

	keys, err = s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: "not-existing"})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 0)
}

func (s *sshKeysSuite) TestCascadeDeleteModel(c *qt.C) {
	ctx := context.Background()
	key := dbmodel.SSHKey{
		Identity:       s.User,
		PublicKey:      []byte("foo"),
		KeyComment:     "bar",
		MD5Fingerprint: "fake-fingerprint",
		Model:          s.Model,
	}
	err := s.Database.AddSSHKey(ctx, &key)
	c.Assert(err, qt.IsNil)
	keys, err := s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 1)
	// check that deleting the model delete the ssh key
	err = s.Database.DeleteModel(ctx, &s.Model)
	c.Assert(err, qt.IsNil)
	keys, err = s.Database.ListSSHKeysForUser(ctx, s.User.Name, db.SSHKeyModelFilter{ModelUUID: s.Model.UUID.String})
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 0)

}

func TestKeyManagerFacade(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshKeysSuite{})
}
