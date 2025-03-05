// Copyright 2025 Canonical.

package ssh_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/juju/names/v5"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/identity"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type sshManagerSuite struct {
	publicKey             sshkeys.PublicKey
	allowedModelUUID      string
	allowedControllerUUID string

	sshManager *ssh.SSHManager

	userWithAccess    *openfga.User
	userWithoutAccess *openfga.User
}

const testSSHManagerEnv = `
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@canonical.com
  type: empty
clouds:
- name: test
  type: test
  regions:
  - name: test-region
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
  public-address: localhost

models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: alice@canonical.com
    access: admin
users:
- username: alice@canonical.com
  controller-access: superuser
`

func (s *sshManagerSuite) Init(c *qt.C) {
	ctx := context.Background()
	uuid := "00000002-0000-0000-0000-000000000001"
	jimmTag := names.NewControllerTag(uuid)
	// Setup DB

	database := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	identityManager, err := identity.NewIdentityManager(database, ofgaClient)
	c.Assert(err, qt.IsNil)

	// this is a mock non-mock model manager, bandaid until we have a real model manager to avoid creating a whole jimm.
	modelManager := mocks.ModelManager{
		GetModel_: func(ctx context.Context, uuid string) (dbmodel.Model, error) {
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: uuid,
					Valid:  true,
				},
			}
			err := database.GetModel(ctx, &m)
			return m, err
		},
	}
	permissionManager, err := permissions.NewManager(database, ofgaClient, uuid, jimmTag)
	c.Assert(err, qt.IsNil)
	jwtFactory := jujuauth.NewFactory(database, mocks.JWTService{
		NewJWT_: func(ctx context.Context, j jimmjwx.JWTParams) ([]byte, error) {
			return []byte("jwt"), nil
		},
	}, permissionManager)

	sshKeyManager, err := sshkeys.NewSSHKeyManager(database)
	c.Assert(err, qt.IsNil)

	s.sshManager, err = ssh.NewSSHManager(identityManager, &modelManager, sshKeyManager, jwtFactory)
	c.Assert(err, qt.IsNil)
	env := jimmtest.ParseEnvironment(c, testSSHManagerEnv)
	env.PopulateDB(c, database)
	env.PopulateDBAndPermissions(c, jimmTag, database, ofgaClient)
	// create a user and set permission for one model
	s.userWithAccess, err = identityManager.FetchIdentity(ctx, env.Users[0].Username)
	c.Assert(err, qt.IsNil)
	s.allowedModelUUID = env.Models[0].UUID
	s.allowedControllerUUID = env.Controllers[0].UUID

	// create a user without access
	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	c.Assert(database.DB.Create(i2).Error, qt.IsNil)
	s.userWithoutAccess = openfga.NewUser(i2, ofgaClient)
	// setup public key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)

	pubKey, err := gossh.NewPublicKey(&key.PublicKey)
	c.Assert(err, qt.IsNil)
	s.publicKey = sshkeys.PublicKey{PublicKey: pubKey, Comment: "myComment"}

	c.Assert(err, qt.IsNil)
	err = sshKeyManager.AddUserPublicKey(ctx, s.userWithAccess, db.SSHKeyModelFilter{ModelUUID: s.allowedModelUUID}, s.publicKey)
	c.Assert(err, qt.IsNil)
}

func (s *sshManagerSuite) TestPublicKeyHandler(c *qt.C) {
	ctx := context.Background()

	// Test that the PublicKeyHandler returns the correct user when the public key is valid.
	user, err := s.sshManager.PublicKeyHandler(ctx, s.userWithAccess.Name, s.publicKey.Marshal())
	c.Assert(err, qt.IsNil)
	c.Assert(user.Identity.Name, qt.Equals, "alice@canonical.com")

	// Test that the PublicKeyHandler returns an error when the public key is invalid.
	_, err = s.sshManager.PublicKeyHandler(ctx, s.userWithoutAccess.Name, s.publicKey.Marshal())
	c.Assert(err, qt.ErrorMatches, `cannot verify key for user`)
}

func (s *sshManagerSuite) TestControllerInfoFromModelUUID(c *qt.C) {
	ctx := context.Background()

	// Test that the ControllerInfoFromModelUUID returns the correct controller address and user when the model UUID is valid.
	connInfo, err := s.sshManager.ControllerInfoFromModelUUID(ctx, s.allowedModelUUID, s.userWithAccess)
	c.Assert(err, qt.IsNil)
	c.Assert(connInfo.Addresses, qt.HasLen, 1)
	c.Assert(connInfo.JWT, qt.Not(qt.HasLen), 0)

	// Test that the ControllerInfoFromModelUUID returns an error when the model UUID is invalid.
	_, err = s.sshManager.ControllerInfoFromModelUUID(ctx, "not-valid", s.userWithAccess)
	c.Assert(err, qt.ErrorMatches, ".*cannot find model.*")
}

func TestSSHManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshManagerSuite{})
}
