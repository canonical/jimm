// Copyright 2025 Canonical.

package permissions_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type permissionManagerSuite struct {
	manager    *permissions.PermissionManager
	adminUser  *openfga.User
	user       *openfga.User
	db         *db.Database
	ctlTag     names.ControllerTag
	ofgaClient *openfga.OFGAClient
}

func (s *permissionManagerSuite) Init(c *qt.C) {
	ctx := context.Background()

	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	uuid := uuid.New()
	ctlTag := names.NewControllerTag(uuid.String())
	s.ctlTag = ctlTag

	s.manager, err = permissions.NewManager(db, ofgaClient, uuid.String(), ctlTag)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.adminUser = openfga.NewUser(i, ofgaClient)
	s.adminUser.JimmAdmin = true

	err = s.adminUser.SetControllerAccess(ctx, ctlTag, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i2, ofgaClient)
}

func TestPermissionManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &permissionManagerSuite{})
}
