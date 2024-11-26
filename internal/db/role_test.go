// Copyright 2024 Canonical.

package db_test

import (
	"context"
	"fmt"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func (s *dbSuite) TestAddRole(c *qt.C) {
	ctx := context.Background()

	uuid := uuid.NewString()
	c.Patch(db.NewUUID, func() string {
		return uuid
	})

	_, err := s.Database.AddRole(ctx, "test-role")
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	roleEntry, err := s.Database.AddRole(ctx, "test-role")
	c.Assert(err, qt.IsNil)
	c.Assert(roleEntry.UUID, qt.Not(qt.Equals), "")

	_, err = s.Database.AddRole(ctx, "test-role")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)

	re := dbmodel.RoleEntry{
		Name: "test-role",
	}
	tx := s.Database.DB.First(&re)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(re.ID, qt.Equals, uint(1))
	c.Assert(re.Name, qt.Equals, "test-role")
	c.Assert(re.UUID, qt.Equals, uuid)
}

func (s *dbSuite) TestGetRole(c *qt.C) {
	uuid1 := uuid.NewString()
	c.Patch(db.NewUUID, func() string {
		return uuid1
	})

	err := s.Database.GetRole(context.Background(), &dbmodel.RoleEntry{})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	role := &dbmodel.RoleEntry{}
	err = s.Database.GetRole(context.Background(), role)
	c.Check(err, qt.ErrorMatches, "must specify uuid or name")

	re1, err := s.Database.AddRole(context.TODO(), "test-role")
	c.Assert(err, qt.IsNil)
	c.Assert(re1.UUID, qt.Equals, uuid1)

	// Get by UUID
	re2 := &dbmodel.RoleEntry{
		UUID: uuid1,
	}
	err = s.Database.GetRole(context.Background(), re2)
	c.Assert(err, qt.IsNil)
	c.Assert(re1, jimmtest.DBObjectEquals, re2)

	// Get by name
	re3 := &dbmodel.RoleEntry{
		Name: "test-role",
	}
	err = s.Database.GetRole(context.Background(), re3)
	c.Assert(err, qt.IsNil)
	c.Assert(re1, jimmtest.DBObjectEquals, re3)
}

func (s *dbSuite) TestUpdateRoleName(c *qt.C) {
	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateRoleName(context.Background(), "blah", "blah")
	c.Check(err, qt.ErrorMatches, "role not found")
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.UpdateRoleName(context.Background(), "", "")
	c.Check(err, qt.ErrorMatches, "name must be specified")

	_, err = s.Database.AddRole(context.Background(), "test-role")
	c.Assert(err, qt.IsNil)

	re1 := &dbmodel.RoleEntry{
		Name: "test-role",
	}
	err = s.Database.GetRole(context.Background(), re1)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateRoleName(context.Background(), re1.Name, "renamed-role")
	c.Check(err, qt.IsNil)

	re2 := &dbmodel.RoleEntry{
		UUID: re1.UUID,
	}
	err = s.Database.GetRole(context.Background(), re2)
	c.Check(err, qt.IsNil)
	c.Assert(re2.Name, qt.Equals, "renamed-role")
}

func (s *dbSuite) TestRemoveRole(c *qt.C) {
	err := s.Database.RemoveRole(context.Background(), &dbmodel.RoleEntry{Name: "test-role"})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	re := &dbmodel.RoleEntry{
		Name: "test-role",
	}
	err = s.Database.RemoveRole(context.Background(), re)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	roleEntry, err := s.Database.AddRole(context.Background(), re.Name)
	c.Assert(err, qt.IsNil)

	ge1 := &dbmodel.RoleEntry{
		Name: "test-role",
	}
	err = s.Database.GetRole(context.Background(), ge1)
	c.Assert(err, qt.IsNil)
	c.Assert(roleEntry.UUID, qt.Equals, ge1.UUID)

	err = s.Database.RemoveRole(context.Background(), ge1)
	c.Check(err, qt.IsNil)

	err = s.Database.GetRole(context.Background(), ge1)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestListRole(c *qt.C) {
	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	addNRoles := 10
	for i := range addNRoles {
		_, err := s.Database.AddRole(context.Background(), fmt.Sprintf("test-role-%d", i))
		c.Assert(err, qt.IsNil)
	}
	ctx := context.Background()
	firstRoles, err := s.Database.ListRoles(ctx, 5, 0, "")
	c.Assert(err, qt.IsNil)
	for i := 0; i < 5; i++ {
		c.Assert(firstRoles[i].Name, qt.Equals, fmt.Sprintf("test-role-%d", i))
	}
	secondRoles, err := s.Database.ListRoles(ctx, 5, 5, "")
	c.Assert(err, qt.IsNil)
	for i := 0; i < 5; i++ {
		c.Assert(secondRoles[i].Name, qt.Equals, fmt.Sprintf("test-role-%d", i+5))
	}

	matchedRoles, err := s.Database.ListRoles(ctx, 5, 0, "role-1")
	c.Assert(err, qt.IsNil)
	c.Assert(matchedRoles, qt.HasLen, 1)
	c.Assert(matchedRoles[0].Name, qt.Equals, "test-role-1")

	matchedRoles, err = s.Database.ListRoles(ctx, 5, 0, "%not-existing%")
	c.Assert(err, qt.IsNil)
	c.Assert(matchedRoles, qt.HasLen, 0)

	tg, err := s.Database.AddRole(context.Background(), "\\%test-role")
	c.Assert(err, qt.IsNil)

	matchedRoles, err = s.Database.ListRoles(ctx, 5, 0, "\\%t")
	c.Assert(err, qt.IsNil)
	c.Assert(matchedRoles, qt.HasLen, 1)
	c.Assert(matchedRoles[0].UUID, qt.Equals, tg.UUID)

	matchedRoles, err = s.Database.ListRoles(ctx, 5, 0, tg.UUID)
	c.Assert(err, qt.IsNil)
	c.Assert(matchedRoles, qt.HasLen, 1)
	c.Assert(matchedRoles[0].UUID, qt.Equals, tg.UUID)
}

func (s *dbSuite) TestCountRoles(c *qt.C) {
	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	addNRoles := 10
	for i := range addNRoles {
		_, err := s.Database.AddRole(context.Background(), fmt.Sprintf("test-role-%d", i))
		c.Assert(err, qt.IsNil)
	}
	count, err := s.Database.CountRoles(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, addNRoles)
}
