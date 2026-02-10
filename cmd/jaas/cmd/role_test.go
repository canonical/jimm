// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"testing"

	"github.com/canonical/jimm/v3/pkg/api/params"
	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"
)

func TestAddRole(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &addRoleCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "myrole")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		AddRole(gomock.Any()).
		DoAndReturn(func(req *params.AddRoleRequest) (params.AddRoleResponse, error) {
			c.Check(req.Name, qt.Equals, "myrole")
			return params.AddRoleResponse{
				Role: params.Role{
					Name: "myrole",
				},
			}, nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), qt.Contains, "myrole")
}

func TestRenameRole(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &renameRoleCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "myrole", "yourrole")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		RenameRole(gomock.Any()).
		DoAndReturn(func(req *params.RenameRoleRequest) error {
			c.Check(req.Name, qt.Equals, "myrole")
			return nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemoveRole(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &removeRoleCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "myrole", "-y")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		RemoveRole(gomock.Any()).
		DoAndReturn(func(req *params.RemoveRoleRequest) error {
			c.Check(req.Name, qt.Equals, "myrole")
			return nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestListRoles(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &listRolesCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "--limit", "10", "--offset", "5")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		ListRoles(gomock.Any()).
		DoAndReturn(func(req *params.ListRolesRequest) ([]params.Role, error) {
			c.Check(req.Limit, qt.Equals, 10)
			c.Check(req.Offset, qt.Equals, 5)
			return []params.Role{
				{Name: "myrole"},
				{Name: "yourrole"},
			}, nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, qt.Contains, "myrole")
	c.Assert(output, qt.Contains, "yourrole")
}
