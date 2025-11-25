// Copyright 2025 Canonical.

package ssh_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	gliderssh "github.com/gliderlabs/ssh"
	jujucontroller "github.com/juju/juju/controller"
	jujutesting "github.com/juju/juju/testing"
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
	publicKey        sshkeys.PublicKey
	allowedModelUUID string
	database         *db.Database
	mockDialer       *mockDialer

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
  public-address: localhost:1234

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

	s.database = &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := s.database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	identityManager, err := identity.NewIdentityManager(s.database, ofgaClient)
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
			err := s.database.GetModel(ctx, &m)
			return m, err
		},
	}
	attrs := map[string]interface{}{
		"ssh-server-port": "17023",
	}
	cfg, err := jujucontroller.NewConfig(uuid, jujutesting.CACert, attrs)
	c.Assert(err, qt.IsNil)
	controllerService := mocks.ControllerService{
		ControllerConfig_: func(ctx context.Context, controllerName string) (jujucontroller.Config, error) {
			return cfg, nil
		},
	}
	jujuManager := mocks.JujuManager{
		ModelManager:      modelManager,
		ControllerService: controllerService,
	}
	permissionManager, err := permissions.NewManager(s.database, ofgaClient, uuid, jimmTag)
	c.Assert(err, qt.IsNil)
	jwtFactory := jujuauth.NewFactory(s.database, mocks.JWTService{
		NewJWT_: func(ctx context.Context, j jimmjwx.JWTParams) ([]byte, error) {
			return []byte("jwt"), nil
		},
	}, permissionManager)

	sshKeyManager, err := sshkeys.NewSSHKeyManager(s.database)
	c.Assert(err, qt.IsNil)

	s.mockDialer = &mockDialer{}

	params := ssh.SSHManagerParams{
		IdentityManager: identityManager,
		JujuManager:     &jujuManager,
		SSHKeyManager:   sshKeyManager,
		JWTFactory:      jwtFactory,
		Dialer:          s.mockDialer,
	}
	s.sshManager, err = ssh.NewSSHManager(params)
	c.Assert(err, qt.IsNil)
	env := jimmtest.ParseEnvironment(c, testSSHManagerEnv)
	env.PopulateDB(c, s.database)
	env.PopulateDBAndPermissions(c, jimmTag, s.database, ofgaClient)
	// create a user and set permission for one model
	s.userWithAccess, err = identityManager.FetchIdentity(ctx, env.Users[0].Username)
	c.Assert(err, qt.IsNil)
	s.allowedModelUUID = env.Models[0].UUID

	// create a user without access
	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	c.Assert(s.database.DB.Create(i2).Error, qt.IsNil)
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
	c.Assert(user.Name, qt.Equals, "alice@canonical.com")

	// Test that the PublicKeyHandler returns an error when the public key is invalid.
	_, err = s.sshManager.PublicKeyHandler(ctx, s.userWithoutAccess.Name, s.publicKey.Marshal())
	c.Assert(err, qt.ErrorMatches, `cannot verify key for user bob: cannot find a matching key for this user`)
}

func (s *sshManagerSuite) TestDialInfo(c *qt.C) {
	ctx := context.Background()

	ctrl := dbmodel.Controller{Name: "test"}
	err := s.database.GetController(ctx, &ctrl)
	c.Assert(err, qt.IsNil)
	c.Assert(ctrl.PublicAddress, qt.Equals, "localhost:1234")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)
	pubKey, err := gossh.NewPublicKey(&key.PublicKey)
	c.Assert(err, qt.IsNil)
	ctx = context.WithValue(ctx, gliderssh.ContextKeyPublicKey, pubKey)

	// Test that the DialInfo returns the correct controller address and user when the model UUID is valid.
	connInfo, err := s.sshManager.DialInfo(ctx, s.allowedModelUUID, s.userWithAccess)
	c.Assert(err, qt.IsNil)
	c.Assert(connInfo.Addresses, qt.HasLen, 1)
	c.Assert(connInfo.Addresses[0], qt.Equals, "localhost")
	c.Assert(connInfo.JWT, qt.Not(qt.HasLen), 0)
	c.Assert(connInfo.Port, qt.Equals, 17023)
	_, err = base64.StdEncoding.DecodeString(connInfo.JWT)
	c.Assert(err, qt.IsNil)

	// Test that the ControllerInfoFromModelUUID returns an error when the model UUID is invalid.
	_, err = s.sshManager.DialInfo(ctx, "not-valid", s.userWithAccess)
	c.Assert(err, qt.ErrorMatches, ".*cannot find model.*")
}

type mockDialer struct {
	validAddress string
	callCount    int
}

func (d *mockDialer) Dial(network string, addr string, config *gossh.ClientConfig) (*gossh.Client, error) {
	d.callCount++
	if addr == d.validAddress {
		return &gossh.Client{}, nil
	}
	return nil, errors.New("dial error")
}

func (s *sshManagerSuite) TestDialAllAddresses(c *qt.C) {
	ctx := context.Background()

	dialInfo := ssh.DialInfo{
		Addresses: []string{"10.1.2.3", "10.1.2.4"},
		Port:      1234,
		JWT:       "fake-jwt",
	}

	_, err := s.sshManager.DialController(ctx, dialInfo, s.userWithAccess)
	c.Assert(err, qt.ErrorMatches, "failed to dial controller: dial error\ndial error")
	c.Assert(s.mockDialer.callCount, qt.Equals, 2)

	s.mockDialer.validAddress = "10.1.2.4:1234"
	// Test that DialController works when there are multiple addresses.
	_, err = s.sshManager.DialController(ctx, dialInfo, s.userWithAccess)
	c.Assert(err, qt.IsNil)
	c.Assert(s.mockDialer.callCount, qt.Equals, 4)
}

func TestSSHManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshManagerSuite{})
}
