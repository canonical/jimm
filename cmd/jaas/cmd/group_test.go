// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestAddGroup(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	// Setup expectations
	expectedGroup := params.Group{
		UUID: "group-uuid",
		Name: "test-group",
	}
	s.client.EXPECT().AddGroup(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, agr *params.AddGroupRequest) (params.AddGroupResponse, error) {
		c.Check(agr.Name, qt.Equals, "test-group")
		return params.AddGroupResponse{Group: expectedGroup}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	// Create command with mocked dependencies
	command := &addGroupCommand{}
	command.SetClientStore(s.store)
	command.setJIMMAPI(s.client)
	initCommand(c, command, "test-group")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	yamlResp := ctx.Stdout.(*bytes.Buffer).String()
	resp := params.AddGroupResponse{}
	yamlErr := yaml.Unmarshal([]byte(yamlResp), &resp)
	c.Assert(yamlErr, qt.IsNil)
	c.Assert(resp.Group, qt.DeepEquals, expectedGroup)
}

func TestRenameGroup(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	// Setup expectations
	s.client.EXPECT().RenameGroup(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, rgr *params.RenameGroupRequest) error {
		c.Check(rgr.Name, qt.Equals, "old-group")
		c.Check(rgr.NewName, qt.Equals, "new-group")
		return nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &renameGroupCommand{
		name:    "old-group",
		newName: "new-group",
	}

	command.SetClientStore(s.store)
	command.setJIMMAPI(s.client)
	initCommand(c, command, "old-group", "new-group")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemoveGroup(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	// Setup expectations
	s.client.EXPECT().RemoveGroup(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, rgr *params.RemoveGroupRequest) error {
		c.Check(rgr.Name, qt.Equals, "test-group")
		return nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &removeGroupCommand{
		name: "test-group",
	}
	command.setJIMMAPI(s.client)

	initCommand(c, command, "test-group")
	ctx := newTestContext(c)
	ctx.Stdin = bytes.NewBufferString("y\n")
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemoveGroupForce(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	// Setup expectations
	s.client.EXPECT().RemoveGroup(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, rgr *params.RemoveGroupRequest) error {
		c.Check(rgr.Name, qt.Equals, "test-group")
		return nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &removeGroupCommand{
		name: "test-group",
	}
	command.setJIMMAPI(s.client)

	initCommand(c, command, "test-group", "--force")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestListGroups(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	// Setup expectations
	s.client.EXPECT().ListGroups(gomock.Any(), gomock.Any()).Return([]params.Group{
		{Name: "group-1", UUID: "uuid-1"},
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &listGroupsCommand{}
	command.SetClientStore(s.store)
	command.setJIMMAPI(s.client)
	initCommand(c, command)

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}
