// Copyright 2025 Canonical.

package serviceaccount_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

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

func (s *serviceAccountManagerSuite) Init(c *qt.C) {
	j := jimmtest.NewJIMM(c, nil)
	s.db = j.Database
	s.ofgaClient = j.OpenFGAClient

	var err error
	s.manager, err = serviceaccount.NewServiceAccountManager(j.Database, j.OpenFGAClient, j)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i, j.OpenFGAClient)
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
