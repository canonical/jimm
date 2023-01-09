// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

type groupSuite struct {
	jimmSuite
}

var _ = gc.Suite(&groupSuite{})

func (s *groupSuite) TestAddGroupSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore, bClient), "test-group")
	c.Assert(err, gc.IsNil)

	group, err := s.jimmSuite.JIMM.Database.GetGroup(context.TODO(), "test-group")
	c.Assert(err, gc.IsNil)
	c.Assert(group.ID, gc.Equals, uint(1))
	c.Assert(group.Name, gc.Equals, "test-group")
}

func (s *groupSuite) TestAddGroup(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore, bClient), "test-group")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *groupSuite) TestRenameGroupSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")

	err := s.jimmSuite.JIMM.Database.AddGroup(context.TODO(), "test-group")
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRenameGroupCommandForTesting(s.ClientStore, bClient), "test-group", "renamed-group")
	c.Assert(err, gc.IsNil)

	group, err := s.jimmSuite.JIMM.Database.GetGroup(context.TODO(), "renamed-group")
	c.Assert(err, gc.IsNil)
	c.Assert(group.ID, gc.Equals, uint(1))
	c.Assert(group.Name, gc.Equals, "renamed-group")
}

func (s *groupSuite) TestRenameGroup(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRenameGroupCommandForTesting(s.ClientStore, bClient), "test-group", "renamed-group")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *groupSuite) TestRemoveGroupSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")

	err := s.jimmSuite.JIMM.Database.AddGroup(context.TODO(), "test-group")
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveGroupCommandForTesting(s.ClientStore, bClient), "test-group")
	c.Assert(err, gc.IsNil)

	_, err = s.jimmSuite.JIMM.Database.GetGroup(context.TODO(), "test-group")
	c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *groupSuite) TestRemoveGroup(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveGroupCommandForTesting(s.ClientStore, bClient), "test-group")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
