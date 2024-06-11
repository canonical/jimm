// Copyright 2021 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

func TestAddGroupUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddGroup(context.Background(), "test-group")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestAddGroup(c *qt.C) {
	ctx := context.Background()

	uuid := uuid.NewString()
	c.Patch(db.NewUUID, func() string {
		return uuid
	})

	err := s.Database.AddGroup(ctx, "test-group")
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.AddGroup(ctx, "test-group")
	c.Assert(err, qt.IsNil)

	err = s.Database.AddGroup(ctx, "test-group")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)

	ge := dbmodel.GroupEntry{
		Name: "test-group",
	}
	tx := s.Database.DB.First(&ge)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(ge.ID, qt.Equals, uint(1))
	c.Assert(ge.Name, qt.Equals, "test-group")
	c.Assert(ge.UUID, qt.Equals, uuid)
}

func (s *dbSuite) TestGetGroup(c *qt.C) {
	uuid1 := uuid.NewString()
	c.Patch(db.NewUUID, func() string {
		return uuid1
	})

	err := s.Database.GetGroup(context.Background(), &dbmodel.GroupEntry{
		Name: "test-group",
	})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	group := &dbmodel.GroupEntry{
		Name: "test-group",
	}
	err = s.Database.GetGroup(context.Background(), group)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.AddGroup(context.TODO(), "test-group")
	c.Assert(err, qt.IsNil)

	err = s.Database.GetGroup(context.Background(), group)
	c.Check(err, qt.IsNil)
	c.Assert(group.ID, qt.Equals, uint(1))
	c.Assert(group.Name, qt.Equals, "test-group")
	c.Assert(group.UUID, qt.Equals, uuid1)

	uuid2 := uuid.NewString()
	c.Patch(db.NewUUID, func() string {
		return uuid2
	})

	err = s.Database.AddGroup(context.Background(), "test-group1")
	c.Assert(err, qt.IsNil)

	group = &dbmodel.GroupEntry{
		Name: "test-group1",
	}

	err = s.Database.GetGroup(context.Background(), group)
	c.Check(err, qt.IsNil)
	c.Assert(group.ID, qt.Equals, uint(2))
	c.Assert(group.Name, qt.Equals, "test-group1")
	c.Assert(group.UUID, qt.Equals, uuid2)
}

func (s *dbSuite) TestUpdateGroup(c *qt.C) {
	err := s.Database.UpdateGroup(context.Background(), &dbmodel.GroupEntry{Name: "test-group"})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	ge := &dbmodel.GroupEntry{
		Name: "test-group",
	}

	err = s.Database.UpdateGroup(context.Background(), ge)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.AddGroup(context.Background(), "test-group")
	c.Assert(err, qt.IsNil)

	ge1 := &dbmodel.GroupEntry{
		Name: "test-group",
	}
	err = s.Database.GetGroup(context.Background(), ge1)
	c.Assert(err, qt.IsNil)

	ge1.Name = "renamed-group"
	err = s.Database.UpdateGroup(context.Background(), ge1)
	c.Check(err, qt.IsNil)

	ge2 := &dbmodel.GroupEntry{
		Name: "renamed-group",
	}
	err = s.Database.GetGroup(context.Background(), ge2)
	c.Check(err, qt.IsNil)
	c.Assert(ge2, qt.DeepEquals, ge1)
}

func (s *dbSuite) TestRemoveGroup(c *qt.C) {
	err := s.Database.RemoveGroup(context.Background(), &dbmodel.GroupEntry{Name: "test-group"})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	ge := &dbmodel.GroupEntry{
		Name: "test-group",
	}
	err = s.Database.RemoveGroup(context.Background(), ge)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.AddGroup(context.Background(), ge.Name)
	c.Assert(err, qt.IsNil)

	ge1 := &dbmodel.GroupEntry{
		Name: "test-group",
	}
	err = s.Database.GetGroup(context.Background(), ge1)
	c.Assert(err, qt.IsNil)

	err = s.Database.RemoveGroup(context.Background(), ge1)
	c.Check(err, qt.IsNil)

	err = s.Database.GetGroup(context.Background(), ge1)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}
