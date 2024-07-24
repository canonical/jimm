// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
)

type groupSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&groupSuite{})

func (s *groupSuite) TestAddGroupSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), "test-group")
	c.Assert(err, gc.IsNil)

	group := &dbmodel.GroupEntry{Name: "test-group"}
	err = s.JimmCmdSuite.JIMM.Database.GetGroup(context.TODO(), group)
	c.Assert(err, gc.IsNil)
	c.Assert(group.ID, gc.Equals, uint(1))
	c.Assert(group.Name, gc.Equals, "test-group")
	c.Assert(group.UUID, gc.Not(gc.Equals), "")
}

func (s *groupSuite) TestAddGroup(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), "test-group")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *groupSuite) TestRenameGroupSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	err := s.JimmCmdSuite.JIMM.Database.AddGroup(context.TODO(), "test-group")
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRenameGroupCommandForTesting(s.ClientStore(), bClient), "test-group", "renamed-group")
	c.Assert(err, gc.IsNil)

	group := &dbmodel.GroupEntry{Name: "renamed-group"}
	err = s.JimmCmdSuite.JIMM.Database.GetGroup(context.TODO(), group)
	c.Assert(err, gc.IsNil)
	c.Assert(group.ID, gc.Equals, uint(1))
	c.Assert(group.Name, gc.Equals, "renamed-group")
}

func (s *groupSuite) TestRenameGroup(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRenameGroupCommandForTesting(s.ClientStore(), bClient), "test-group", "renamed-group")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *groupSuite) TestRemoveGroupSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	err := s.JimmCmdSuite.JIMM.Database.AddGroup(context.TODO(), "test-group")
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveGroupCommandForTesting(s.ClientStore(), bClient), "test-group", "-y")
	c.Assert(err, gc.IsNil)

	group := &dbmodel.GroupEntry{Name: "test-group"}
	err = s.JimmCmdSuite.JIMM.Database.GetGroup(context.TODO(), group)
	c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *groupSuite) TestRemoveGroupWithoutFlag(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveGroupCommandForTesting(s.ClientStore(), bClient), "test-group")
	c.Assert(err.Error(), gc.Matches, "Failed to read from input.")
}

func (s *groupSuite) TestRemoveGroup(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveGroupCommandForTesting(s.ClientStore(), bClient), "test-group", "-y")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *groupSuite) TestListGroupsSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	for i := 0; i < 3; i++ {
		err := s.JimmCmdSuite.JIMM.Database.AddGroup(context.TODO(), fmt.Sprint("test-group", i))
		c.Assert(err, gc.IsNil)
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewListGroupsCommandForTesting(s.ClientStore(), bClient), "test-group")
	c.Assert(err, gc.IsNil)
	output := cmdtesting.Stdout(ctx)
	c.Assert(strings.Contains(output, "test-group0"), gc.Equals, true)
	c.Assert(strings.Contains(output, "test-group1"), gc.Equals, true)
	c.Assert(strings.Contains(output, "test-group2"), gc.Equals, true)
}

func (s *groupSuite) TestListGroupsLimitSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	for i := 0; i < 3; i++ {
		err := s.JimmCmdSuite.JIMM.Database.AddGroup(context.TODO(), fmt.Sprint("test-group", i))
		c.Assert(err, gc.IsNil)
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewListGroupsCommandForTesting(s.ClientStore(), bClient), "test-group", "--limit", "1", "--offset", "1")
	c.Assert(err, gc.IsNil)
	output := cmdtesting.Stdout(ctx)
	groups := []params.Group{}
	err = yaml.Unmarshal([]byte(output), &groups)
	c.Assert(err, gc.IsNil)
	c.Assert(groups, gc.HasLen, 1)
	c.Assert(groups[0].Name, gc.Equals, "test-group1")
	c.Assert(groups[0].UUID, gc.Not(gc.Equals), "")
}

func (s *groupSuite) TestListGroups(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewListGroupsCommandForTesting(s.ClientStore(), bClient), "test-group")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
