// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestAddGroup(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	// Setup expectations
	expectedGroup := params.Group{
		UUID: "group-uuid",
		Name: "test-group",
	}
	s.client.EXPECT().AddGroup(gomock.Any()).DoAndReturn(func(agr *params.AddGroupRequest) (params.AddGroupResponse, error) {
		c.Check(agr.Name, qt.Equals, "test-group")
		return params.AddGroupResponse{Group: expectedGroup}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	// Create command with mocked dependencies
	command := &addGroupCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	command.SetClientStore(s.store)
	initCommand(c, command, "test-group")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	yamlResp := ctx.Stdout.(*bytes.Buffer).String()
	resp := params.AddGroupResponse{}
	yamlErr := yaml.Unmarshal([]byte(yamlResp), &resp)
	c.Assert(yamlErr, qt.IsNil)
	c.Assert(resp.Group, qt.DeepEquals, expectedGroup)
}

func TestAddGroupAPIError(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	expectedErr := errors.New("failed to connect")

	command := &addGroupCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return nil, expectedErr
		},
	}

	command.SetClientStore(s.store)
	initCommand(c, command, "test-group")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNotNil)
}

func TestRenameGroup(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	// Setup expectations
	s.client.EXPECT().RenameGroup(gomock.Any()).DoAndReturn(func(rgr *params.RenameGroupRequest) error {
		c.Check(rgr.Name, qt.Equals, "old-group")
		c.Check(rgr.NewName, qt.Equals, "new-group")
		return nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &renameGroupCommand{
		name:    "old-group",
		newName: "new-group",
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	command.SetClientStore(s.store)
	initCommand(c, command, "old-group", "new-group")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemoveGroup(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	// Setup expectations
	s.client.EXPECT().RemoveGroup(gomock.Any()).DoAndReturn(func(rgr *params.RemoveGroupRequest) error {
		c.Check(rgr.Name, qt.Equals, "test-group")
		return nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &removeGroupCommand{
		name: "test-group",
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	initCommand(c, command, "test-group")
	ctx := newTestContext(t)
	ctx.Stdin = bytes.NewBufferString("y\n")
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemoveGroupForce(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	// Setup expectations
	s.client.EXPECT().RemoveGroup(gomock.Any()).DoAndReturn(func(rgr *params.RemoveGroupRequest) error {
		c.Check(rgr.Name, qt.Equals, "test-group")
		return nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &removeGroupCommand{
		name: "test-group",
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	initCommand(c, command, "test-group", "--force")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestListGroups(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	// Setup expectations
	s.client.EXPECT().ListGroups(gomock.Any()).Return([]params.Group{
		{Name: "group-1", UUID: "uuid-1"},
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &listGroupsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	command.SetClientStore(s.store)
	initCommand(c, command)

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}
