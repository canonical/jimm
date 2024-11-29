// Copyright 2024 Canonical.

package group_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/group"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type groupManagerSuite struct {
	manager    *group.GroupManager
	adminUser  *openfga.User
	user       *openfga.User
	db         *db.Database
	ofgaClient *openfga.OFGAClient
}

func (s *groupManagerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	s.db = db

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.manager, err = group.NewGroupManager(db, ofgaClient)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.adminUser = openfga.NewUser(i, ofgaClient)
	s.adminUser.JimmAdmin = true

	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i2, ofgaClient)
}

func (s *groupManagerSuite) TestAddGroup(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	g, err := s.manager.AddGroup(ctx, s.adminUser, "test-group-1")
	c.Assert(err, qt.IsNil)
	c.Assert(g.UUID, qt.Not(qt.Equals), "")
	c.Assert(g.Name, qt.Equals, "test-group-1")

	g, err = s.manager.AddGroup(ctx, s.adminUser, "test-group-2")
	c.Assert(err, qt.IsNil)
	c.Assert(g.UUID, qt.Not(qt.Equals), "")
	c.Assert(g.Name, qt.Equals, "test-group-2")
}

func (s *groupManagerSuite) TestCountGroups(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	groupEntry, err := s.manager.AddGroup(ctx, s.adminUser, "test-group-1")
	c.Assert(err, qt.IsNil)
	c.Assert(groupEntry.UUID, qt.Not(qt.Equals), "")

	_, err = s.manager.AddGroup(ctx, s.adminUser, "test-group-1")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)
}

func (s *groupManagerSuite) TestGetGroup(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	groupEntry, err := s.manager.AddGroup(ctx, s.adminUser, "test-group-1")
	c.Assert(err, qt.IsNil)
	c.Assert(groupEntry.UUID, qt.Not(qt.Equals), "")

	gotGroupUuid, err := s.manager.GetGroupByUUID(ctx, s.adminUser, groupEntry.UUID)
	c.Assert(err, qt.IsNil)
	c.Assert(gotGroupUuid, qt.DeepEquals, groupEntry)

	gotGroupName, err := s.manager.GetGroupByName(ctx, s.adminUser, groupEntry.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(gotGroupName, qt.DeepEquals, groupEntry)

	_, err = s.manager.GetGroupByUUID(ctx, s.adminUser, "non-existent")
	c.Assert(err, qt.Not(qt.IsNil))

	_, err = s.manager.GetGroupByName(ctx, s.adminUser, "non-existent")
	c.Assert(err, qt.Not(qt.IsNil))
}

func (s *groupManagerSuite) TestRemoveGroup(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	_, group, _, _, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, *s.db)

	err := s.manager.RemoveGroup(ctx, s.adminUser, group.Name)
	c.Assert(err, qt.IsNil)

	err = s.manager.RemoveGroup(ctx, s.adminUser, group.Name)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *groupManagerSuite) TestRemoveGroupRemovesTuples(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	user, group, controller, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, *s.db)

	_, err := s.db.AddGroup(ctx, "test-group2")
	c.Assert(err, qt.IsNil)

	group2 := &dbmodel.GroupEntry{
		Name: "test-group2",
	}
	err = s.db.GetGroup(ctx, group2)
	c.Assert(err, qt.IsNil)

	tuples := []openfga.Tuple{
		// This tuple should remain as it has no relation to group2
		{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: "member",
			Target:   ofganames.ConvertTag(group.ResourceTag()),
		},
		// Below tuples should all be removed as they relate to group2
		{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: "member",
			Target:   ofganames.ConvertTag(group2.ResourceTag()),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(group2.ResourceTag(), ofganames.MemberRelation),
			Relation: "member",
			Target:   ofganames.ConvertTag(group.ResourceTag()),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(group2.ResourceTag(), ofganames.MemberRelation),
			Relation: "administrator",
			Target:   ofganames.ConvertTag(controller.ResourceTag()),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(group2.ResourceTag(), ofganames.MemberRelation),
			Relation: "writer",
			Target:   ofganames.ConvertTag(model.ResourceTag()),
		},
	}

	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	err = s.manager.RemoveGroup(ctx, s.adminUser, group.Name)
	c.Assert(err, qt.IsNil)

	err = s.manager.RemoveGroup(ctx, s.adminUser, group.Name)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	remainingTuples, _, err := s.ofgaClient.ReadRelatedObjects(ctx, ofga.Tuple{}, 0, "")
	c.Assert(err, qt.IsNil)
	c.Assert(remainingTuples, qt.HasLen, 3)

	err = s.manager.RemoveGroup(ctx, s.adminUser, group2.Name)
	c.Assert(err, qt.IsNil)

	remainingTuples, _, err = s.ofgaClient.ReadRelatedObjects(ctx, ofga.Tuple{}, 0, "")
	c.Assert(err, qt.IsNil)
	c.Assert(remainingTuples, qt.HasLen, 0)
}

func (s *groupManagerSuite) TestRenameGroup(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	user, group, controller, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, *s.db)

	tuples := []openfga.Tuple{
		{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: "member",
			Target:   ofganames.ConvertTag(group.ResourceTag()),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
			Relation: "administrator",
			Target:   ofganames.ConvertTag(controller.ResourceTag()),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
			Relation: "writer",
			Target:   ofganames.ConvertTag(model.ResourceTag()),
		},
	}

	err := s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	err = s.manager.RenameGroup(ctx, s.adminUser, group.Name, "test-new-group")
	c.Assert(err, qt.IsNil)

	group.Name = "test-new-group"

	// check the user still has member relation to the group
	allowed, err := s.ofgaClient.CheckRelation(
		ctx,
		ofga.Tuple{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: "member",
			Target:   ofganames.ConvertTag(group.ResourceTag()),
		},
		false,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsTrue)

	// check the user still has writer relation to the model via the
	// group membership
	allowed, err = s.ofgaClient.CheckRelation(
		ctx,
		ofga.Tuple{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: "writer",
			Target:   ofganames.ConvertTag(model.ResourceTag()),
		},
		false,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsTrue)

	// check the user still has administrator relation to the controller
	// via group membership
	allowed, err = s.ofgaClient.CheckRelation(
		ctx,
		ofga.Tuple{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: "administrator",
			Target:   ofganames.ConvertTag(controller.ResourceTag()),
		},
		false,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsTrue)
}

func (s *groupManagerSuite) TestListGroups(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	user, group, _, _, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, *s.db)

	u := openfga.NewUser(&user, s.ofgaClient)
	u.JimmAdmin = true

	pagination := pagination.NewOffsetFilter(10, 0)
	groups, err := s.manager.ListGroups(ctx, u, pagination, "")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.DeepEquals, []dbmodel.GroupEntry{group})

	groupNames := []string{
		"test-group0",
		"test-group1",
		"test-group2",
		"aaaFinalGroup",
	}

	for _, name := range groupNames {
		_, err := s.manager.AddGroup(ctx, u, name)
		c.Assert(err, qt.IsNil)
	}
	groups, err = s.manager.ListGroups(ctx, u, pagination, "")
	c.Assert(err, qt.IsNil)
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	c.Assert(groups, qt.HasLen, 5)
	// Check that the UUID is not empty
	c.Assert(groups[0].UUID, qt.Not(qt.Equals), "")
	// groups should be returned in ascending order of name
	c.Assert(groups[0].Name, qt.Equals, "aaaFinalGroup")
	c.Assert(groups[1].Name, qt.Equals, group.Name)
	c.Assert(groups[2].Name, qt.Equals, "test-group0")
	c.Assert(groups[3].Name, qt.Equals, "test-group1")
	c.Assert(groups[4].Name, qt.Equals, "test-group2")
}

func TestGroupManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &groupManagerSuite{})
}
