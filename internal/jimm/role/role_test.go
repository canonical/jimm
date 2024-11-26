// Copyright 2024 Canonical.

package role_test

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	cofga "github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/role"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmtest "github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type roleManagerSuite struct {
	manager    *role.RoleManager
	user       *openfga.User
	db         *db.Database
	ofgaClient *openfga.OFGAClient
}

func (s *roleManagerSuite) Init(c *qt.C) {
	// Setup DB
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	s.db = db

	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.manager, err = role.NewRoleManager(db, ofgaClient)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i, ofgaClient)
}

func (s *roleManagerSuite) TestAddRole(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// Check only admin can AddRole
	_, err := s.manager.AddRole(ctx, s.user, "models")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Check happypath
	s.user.JimmAdmin = true

	re, err := s.manager.AddRole(ctx, s.user, "models")
	c.Assert(err, qt.IsNil)

	c.Assert(re, jimmtest.DBObjectEquals, &dbmodel.RoleEntry{
		Name: "models",
		UUID: re.UUID,
		ID:   1,
	})
}

func (s *roleManagerSuite) TestGetRole(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// Check only admin can GetRoleByName (this affects UUID too)
	_, err := s.manager.GetRoleByName(ctx, s.user, "models")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Check happypath
	s.user.JimmAdmin = true

	re, err := s.db.AddRole(ctx, "models")
	c.Assert(err, qt.IsNil)

	re2, err := s.manager.GetRoleByUUID(ctx, s.user, re.UUID)
	c.Assert(err, qt.IsNil)
	c.Assert(re.ID, qt.Equals, re2.ID)

	re3, err := s.manager.GetRoleByName(ctx, s.user, re.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(re.ID, qt.Equals, re3.ID)
}

func (s *roleManagerSuite) TestRemoveRole(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// Check only admin can RemoveRole
	err := s.manager.RemoveRole(ctx, s.user, "r1")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Check happypath
	s.user.JimmAdmin = true

	// Add to the db and openfga.
	re, err := s.db.AddRole(ctx, "r1")
	c.Assert(err, qt.IsNil)

	tupe := openfga.Tuple{
		Object: &cofga.Entity{
			Kind: "user",
			ID:   s.user.Name,
		},
		Relation: ofganames.AssigneeRelation,
		Target: &cofga.Entity{
			Kind: "role",
			ID:   re.UUID,
		},
	}
	err = s.ofgaClient.AddRelation(ctx, tupe)
	c.Assert(err, qt.IsNil)
	allowed, err := s.ofgaClient.CheckRelation(ctx, tupe, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	err = s.manager.RemoveRole(ctx, s.user, "r1")
	c.Assert(err, qt.IsNil)

	// Ensure db is cleaned up
	err = s.db.GetRole(ctx, &dbmodel.RoleEntry{Name: "r1"})
	c.Assert(err, qt.ErrorMatches, "record not found")

	// Ensure tuple is gone
	tupes, _, err := s.ofgaClient.ReadRelatedObjects(
		ctx,
		openfga.Tuple{
			Target: &cofga.Entity{
				Kind: "role",
				ID:   re.UUID,
			},
		},
		1,
		"",
	)
	c.Assert(err, qt.IsNil)
	c.Assert(tupes, qt.HasLen, 0)
}

func (s *roleManagerSuite) TestRenameRole(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// Check only admin can RenameRole
	err := s.manager.RenameRole(ctx, s.user, "uuid", "models")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Check happypath
	s.user.JimmAdmin = true

	// Add to the db.
	re, err := s.db.AddRole(ctx, "models")
	c.Assert(err, qt.IsNil)

	err = s.manager.RenameRole(ctx, s.user, re.Name, "models-role")
	c.Assert(err, qt.IsNil)

	updatedRe := &dbmodel.RoleEntry{
		UUID: re.UUID,
	}
	err = s.db.GetRole(ctx, updatedRe)
	c.Assert(err, qt.IsNil)
	c.Assert(updatedRe.Name, qt.Equals, "models-role")
}

func (s *roleManagerSuite) TestListRoles(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// Check only admin can ListRoles
	pag := pagination.NewOffsetFilter(10, 0)
	_, err := s.manager.ListRoles(ctx, s.user, pag, "")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Check happypath
	s.user.JimmAdmin = true

	for i := 0; i < 3; i++ {
		_, err = s.db.AddRole(ctx, "r"+strconv.Itoa(i))
		c.Assert(err, qt.IsNil)
	}

	pag = pagination.NewOffsetFilter(2, 0)
	res, err := s.manager.ListRoles(ctx, s.user, pag, "r")
	c.Assert(err, qt.IsNil)
	sort.Slice(res, func(i, j int) bool { return i < j })
	c.Assert(res, qt.HasLen, 2)
	c.Assert(res[0].Name, qt.Equals, "r0")
	c.Assert(res[1].Name, qt.Equals, "r1")

	pag = pagination.NewOffsetFilter(10, 2)
	res, err = s.manager.ListRoles(ctx, s.user, pag, "r")
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.HasLen, 1)
	c.Assert(res[0].Name, qt.Equals, "r2")
}

func (s *roleManagerSuite) TestCountRoles(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// Check only admin can CountRoles
	_, err := s.manager.CountRoles(ctx, s.user)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Check happypath
	s.user.JimmAdmin = true

	for i := 0; i < 3; i++ {
		_, err = s.db.AddRole(ctx, "r"+strconv.Itoa(i))
		c.Assert(err, qt.IsNil)
	}

	amount, err := s.manager.CountRoles(ctx, s.user)
	c.Assert(err, qt.IsNil)
	c.Assert(amount, qt.Equals, 3)
}

func TestRoleManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &roleManagerSuite{})
}
