// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"os"
	"path"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestAddRelationMissingParams(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &addPermissionCommand{}
	command.SetClientStore(cmdMocks.store)

	err := initCommandWithError(command, "foo", "bar")
	c.Assert(err, qt.ErrorMatches, "target object not specified")

	err = initCommandWithError(command, "foo")
	c.Assert(err, qt.ErrorMatches, "relation not specified")

	err = initCommandWithError(command)
	c.Assert(err, qt.ErrorMatches, "object not specified")
}

func TestAddRelation(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &addPermissionCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "user-alice@canonical.com", "member", "group-mygroup")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		AddRelation(gomock.Any()).
		DoAndReturn(func(req *params.AddRelationRequest) error {
			c.Check(req.Tuples, qt.HasLen, 1)
			tuple := req.Tuples[0]
			c.Check(tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(tuple.Relation, qt.Equals, "member")
			c.Check(tuple.TargetObject, qt.Equals, "group-mygroup")
			return nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestAddRelationFromFile(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &addPermissionCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	tmpFile := makeTempFile(c, "add_permission_test.json", `[{
	  "object": "user-alice@canonical.com",
	  "relation": "member",
	  "target_object": "group-mygroup"
	}]`)

	initCommand(c, command, "-f", tmpFile)

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		AddRelation(gomock.Any()).
		DoAndReturn(func(req *params.AddRelationRequest) error {
			c.Check(req.Tuples, qt.HasLen, 1)
			tuple := req.Tuples[0]
			c.Check(tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(tuple.Relation, qt.Equals, "member")
			c.Check(tuple.TargetObject, qt.Equals, "group-mygroup")
			return nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemovePermission(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &removePermissionCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "user-alice@canonical.com", "member", "group-mygroup")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		RemoveRelation(gomock.Any()).
		DoAndReturn(func(req *params.RemoveRelationRequest) error {
			c.Check(req.Tuples, qt.HasLen, 1)
			tuple := req.Tuples[0]
			c.Check(tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(tuple.Relation, qt.Equals, "member")
			c.Check(tuple.TargetObject, qt.Equals, "group-mygroup")
			return nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestRemoveRelationFromFile(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &removePermissionCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	tmpFile := makeTempFile(c, "add_permission_test.json", `[{
	  "object": "user-alice@canonical.com",
	  "relation": "member",
	  "target_object": "group-mygroup"
	}]`)

	initCommand(c, command, "-f", tmpFile)

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		RemoveRelation(gomock.Any()).
		DoAndReturn(func(req *params.RemoveRelationRequest) error {
			c.Check(req.Tuples, qt.HasLen, 1)
			tuple := req.Tuples[0]
			c.Check(tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(tuple.Relation, qt.Equals, "member")
			c.Check(tuple.TargetObject, qt.Equals, "group-mygroup")
			return nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestCheckPermission(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &checkPermissionCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "user-alice@canonical.com", "member", "group-mygroup")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		CheckRelation(gomock.Any()).
		DoAndReturn(func(req *params.CheckRelationRequest) (params.CheckRelationResponse, error) {
			c.Check(req.Tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(req.Tuple.Relation, qt.Equals, "member")
			c.Check(req.Tuple.TargetObject, qt.Equals, "group-mygroup")
			return params.CheckRelationResponse{
				Allowed: true,
			}, nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), qt.Contains, `is allowed`)
}

func TestListPermissions(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &listPermissionsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "--object", "user-alice@canonical.com", "--relation", "member", "--target", "group-mygroup", "--resolve")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		ListRelationshipTuples(gomock.Any()).
		DoAndReturn(func(req *params.ListRelationshipTuplesRequest) (*params.ListRelationshipTuplesResponse, error) {
			c.Check(req.Tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(req.Tuple.Relation, qt.Equals, "member")
			c.Check(req.Tuple.TargetObject, qt.Equals, "group-mygroup")
			c.Check(req.ResolveUUIDs, qt.Equals, true)
			return &params.ListRelationshipTuplesResponse{
				Tuples: []params.RelationshipTuple{
					{
						Object:       "user-alice@canonical.com",
						Relation:     "member",
						TargetObject: "group-mygroup",
					},
				},
			}, nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), qt.Contains, "user-alice@canonical.com")
}

func TestListPermissionsTabular(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	command := &listPermissionsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "--format", "tabular", "--object", "user-alice@canonical.com", "--relation", "member", "--target", "group-mygroup", "--resolve")

	ctx := newTestContext(c)

	cmdMocks.client.EXPECT().
		ListRelationshipTuples(gomock.Any()).
		DoAndReturn(func(req *params.ListRelationshipTuplesRequest) (*params.ListRelationshipTuplesResponse, error) {
			c.Check(req.Tuple.Object, qt.Equals, "user-alice@canonical.com")
			c.Check(req.Tuple.Relation, qt.Equals, "member")
			c.Check(req.Tuple.TargetObject, qt.Equals, "group-mygroup")
			c.Check(req.ResolveUUIDs, qt.Equals, true)
			return &params.ListRelationshipTuplesResponse{
				Tuples: []params.RelationshipTuple{
					{
						Object:       "user-alice@canonical.com",
						Relation:     "member",
						TargetObject: "group-mygroup",
					},
				},
			}, nil
		}).Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, qt.Contains, "\nuser-alice@canonical.com\tmember  \tgroup-mygroup")
}

func makeTempFile(c *qt.C, name, contents string) string {
	tmpDir := os.TempDir()
	c.Cleanup(func() {
		os.Remove(tmpDir)
	})
	tmpFile := path.Join(os.TempDir(), name)

	err := os.WriteFile(tmpFile, []byte(contents), os.FileMode(0644))
	c.Assert(err, qt.IsNil)

	return tmpFile
}
