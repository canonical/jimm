// Copyright 2025 Canonical.

package serviceaccount_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/serviceaccount"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type serviceAccountManagerSuite struct {
	manager    *serviceaccount.ServiceAccountManager
	user       *openfga.User
	db         *db.Database
	ofgaClient *openfga.OFGAClient
}

type credentialCopier struct{}

func (cc *credentialCopier) CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	newCredID := fmt.Sprintf("%s/%s/%s", cred.Cloud().Id(), newUser.Name, cred.Name())
	newTag := names.NewCloudCredentialTag(newCredID)
	return newTag, nil, nil
}

func (s *serviceAccountManagerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.manager, err = serviceaccount.NewServiceAccountManager(db, ofgaClient, &credentialCopier{})
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i, ofgaClient)
}

func (s *serviceAccountManagerSuite) TestAddServiceAccount(c *qt.C) {
	c.Parallel()

	ctx := context.Background()

	clientID := "39caae91-b914-41ae-83f8-c7b86ca5ad5a@serviceaccount"
	err := s.manager.AddServiceAccount(ctx, s.user, clientID)
	c.Assert(err, qt.IsNil)
	err = s.manager.AddServiceAccount(ctx, s.user, clientID)
	c.Assert(err, qt.IsNil)

	bob, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	userBob := openfga.NewUser(
		bob,
		s.ofgaClient,
	)
	err = s.manager.AddServiceAccount(ctx, userBob, clientID)
	c.Assert(err, qt.ErrorMatches, "service account already owned")
}

func TestServiceAccountManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &serviceAccountManagerSuite{})
}
