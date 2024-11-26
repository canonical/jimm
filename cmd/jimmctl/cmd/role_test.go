// Copyright 2024 Canonical.

package cmd_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type roleSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&roleSuite{})

func (s *roleSuite) TestAddRoleSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	ctx, err := cmdtesting.RunCommand(c, cmd.NewAddRoleCommandForTesting(s.ClientStore(), bClient), "test-role")
	c.Assert(err, gc.IsNil)

	role := &dbmodel.RoleEntry{Name: "test-role"}
	err = s.JimmCmdSuite.JIMM.Database.GetRole(context.Background(), role)
	c.Assert(err, gc.IsNil)
	c.Assert(role.ID, gc.Equals, uint(1))
	c.Assert(role.Name, gc.Equals, "test-role")

	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, fmt.Sprintf(`(?s).*uuid: %s\n.*`, role.UUID))
}

func (s *roleSuite) TestAddRole(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddRoleCommandForTesting(s.ClientStore(), bClient), "test-role")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *roleSuite) TestRenameRoleSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	roleEntry, err := s.JimmCmdSuite.JIMM.Database.AddRole(context.TODO(), "test-role")
	c.Assert(err, gc.IsNil)
	c.Assert(roleEntry.UUID, gc.Not(gc.Equals), "")

	_, err = cmdtesting.RunCommand(c, cmd.NewRenameRoleCommandForTesting(s.ClientStore(), bClient), "test-role", "renamed-role")
	c.Assert(err, gc.IsNil)

	role := &dbmodel.RoleEntry{Name: "renamed-role"}
	err = s.JimmCmdSuite.JIMM.Database.GetRole(context.TODO(), role)
	c.Assert(err, gc.IsNil)
	c.Assert(role.ID, gc.Equals, uint(1))
	c.Assert(role.Name, gc.Equals, "renamed-role")
}

func (s *roleSuite) TestRenameRole(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRenameRoleCommandForTesting(s.ClientStore(), bClient), "test-role", "renamed-role")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *roleSuite) TestRemoveRoleSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	_, err := s.JimmCmdSuite.JIMM.Database.AddRole(context.TODO(), "test-role")
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveRoleCommandForTesting(s.ClientStore(), bClient), "test-role", "-y")
	c.Assert(err, gc.IsNil)

	role := &dbmodel.RoleEntry{Name: "test-role"}
	err = s.JimmCmdSuite.JIMM.Database.GetRole(context.TODO(), role)
	c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *roleSuite) TestRemoveRoleWithoutFlag(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRoleCommandForTesting(s.ClientStore(), bClient), "test-role")
	c.Assert(err.Error(), gc.Matches, "Failed to read from input.")
}

func (s *roleSuite) TestRemoveRole(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRoleCommandForTesting(s.ClientStore(), bClient), "test-role", "-y")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *roleSuite) TestListRolesSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	for i := 0; i < 3; i++ {
		_, err := s.JimmCmdSuite.JIMM.Database.AddRole(context.TODO(), fmt.Sprint("test-role", i))
		c.Assert(err, gc.IsNil)
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewListRolesCommandForTesting(s.ClientStore(), bClient), "test-role")
	c.Assert(err, gc.IsNil)
	output := cmdtesting.Stdout(ctx)
	c.Assert(strings.Contains(output, "test-role0"), gc.Equals, true)
	c.Assert(strings.Contains(output, "test-role1"), gc.Equals, true)
	c.Assert(strings.Contains(output, "test-role2"), gc.Equals, true)
}

func (s *roleSuite) TestListRolesLimitSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	for i := 0; i < 3; i++ {
		_, err := s.JimmCmdSuite.JIMM.Database.AddRole(context.TODO(), fmt.Sprint("test-role", i))
		c.Assert(err, gc.IsNil)
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewListRolesCommandForTesting(s.ClientStore(), bClient), "test-role", "--limit", "1", "--offset", "1")
	c.Assert(err, gc.IsNil)
	output := cmdtesting.Stdout(ctx)
	roles := []params.Role{}
	err = yaml.Unmarshal([]byte(output), &roles)
	c.Assert(err, gc.IsNil)
	c.Assert(roles, gc.HasLen, 1)
	c.Assert(roles[0].Name, gc.Equals, "test-role1")
	c.Assert(roles[0].UUID, gc.Not(gc.Equals), "")
}

func (s *roleSuite) TestListRoles(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewListRolesCommandForTesting(s.ClientStore(), bClient), "test-role")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
