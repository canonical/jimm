// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"database/sql"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	"github.com/canonical/jimm/v3/pkg/names"
)

type accessControlSuite struct {
	websocketSuite
}

var _ = gc.Suite(&accessControlSuite{})

func (s *accessControlSuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)

	// We need to add the default controller, so that
	// we can resolve its tag when listing application offers.
	ctl := dbmodel.Controller{
		Name:      "default_test_controller",
		UUID:      jimmtest.DefaultControllerUUID,
		CloudName: jimmtest.TestCloudName,
	}
	err := s.JIMM.Database.AddController(context.Background(), &ctl)
	c.Assert(err, jc.ErrorIsNil)
}

/*
 Group facade related tests
*/

func (s *accessControlSuite) TestAddGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)
	res, err := client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.UUID, gc.Not(gc.Equals), "")

	_, err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, gc.ErrorMatches, ".*already exists.*")
}

func (s *accessControlSuite) TestGetGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	created, err := client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	retrievedUuid, err := client.GetGroup(&apiparams.GetGroupRequest{UUID: created.UUID})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retrievedUuid.Group, gc.DeepEquals, created.Group)

	retrievedName, err := client.GetGroup(&apiparams.GetGroupRequest{Name: created.Name})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retrievedName.Group, gc.DeepEquals, created.Group)

	_, err = client.GetGroup(&apiparams.GetGroupRequest{UUID: "non-existent"})
	c.Assert(err, gc.ErrorMatches, ".*not found.*")

	_, err = client.GetGroup(&apiparams.GetGroupRequest{Name: created.Name, UUID: created.UUID})
	c.Assert(err, gc.ErrorMatches, ".*only one of.*")
}

func (s *accessControlSuite) TestRemoveGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, gc.ErrorMatches, ".*not found.*")

	_, err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessControlSuite) TestRemoveGroupRemovesTuples(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, group, controller, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	_, err := db.AddGroup(ctx, "test-group2")
	c.Assert(err, gc.IsNil)

	group2 := &dbmodel.GroupEntry{
		Name: "test-group2",
	}
	err = db.GetGroup(ctx, group2)
	c.Assert(err, gc.IsNil)

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

	u := user.Tag().String()

	checkAccessTupleController := apiparams.RelationshipTuple{Object: u, Relation: "administrator", TargetObject: "controller-" + controller.UUID}
	checkAccessTupleModel := apiparams.RelationshipTuple{Object: u, Relation: "writer", TargetObject: "model-" + model.UUID.String}

	err = s.JIMM.OpenFGAClient.AddRelation(context.Background(), tuples...)
	c.Assert(err, gc.IsNil)
	// Check user has access to model and controller through group2
	checkResp, err := client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleController})
	c.Assert(err, gc.IsNil)
	c.Assert(checkResp.Allowed, gc.Equals, true)
	checkResp, err = client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleModel})
	c.Assert(err, gc.IsNil)
	c.Assert(checkResp.Allowed, gc.Equals, true)

	err = client.RemoveGroup(&apiparams.RemoveGroupRequest{Name: group2.Name})
	c.Assert(err, gc.IsNil)

	resp, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{})
	c.Assert(err, gc.IsNil)
	c.Assert(len(resp.Tuples), gc.Equals, 13)

	// Check user access has been revoked.
	checkResp, err = client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleController})
	c.Assert(err, gc.IsNil)
	c.Assert(checkResp.Allowed, gc.Equals, false)
	checkResp, err = client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleModel})
	c.Assert(err, gc.IsNil)
	c.Assert(checkResp.Allowed, gc.Equals, false)
}

func (s *accessControlSuite) TestRenameGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RenameGroup(&apiparams.RenameGroupRequest{
		Name:    "test-group",
		NewName: "renamed-group",
	})
	c.Assert(err, gc.ErrorMatches, ".*not found.*")

	_, err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RenameGroup(&apiparams.RenameGroupRequest{
		Name:    "test-group",
		NewName: "renamed-group",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessControlSuite) TestListGroups(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	groupNames := []string{
		"test-group0",
		"test-group1",
		"test-group2",
		"aaaFinalGroup",
	}

	for _, name := range groupNames {
		_, err := client.AddGroup(&apiparams.AddGroupRequest{Name: name})
		c.Assert(err, jc.ErrorIsNil)
	}
	req := apiparams.ListGroupsRequest{Limit: 10, Offset: 0}
	groups, err := client.ListGroups(&req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groups, gc.HasLen, 4)
	// Verify the UUID is not empty.
	c.Assert(groups[0].UUID, gc.Not(gc.Equals), "")
	// groups should be returned in ascending order of name
	c.Assert(groups[0].Name, gc.Equals, "aaaFinalGroup")
	c.Assert(groups[1].Name, gc.Equals, "test-group0")
	c.Assert(groups[2].Name, gc.Equals, "test-group1")
	c.Assert(groups[3].Name, gc.Equals, "test-group2")
}

/*
 Relation facade related tests
*/

// createTuple wraps the underlying ofga tuple into a convenient ease-of-use method
func createTuple(object, relation, target string) openfga.Tuple {
	objectEntity, _ := openfga.ParseTag(object)
	targetEntity, _ := openfga.ParseTag(target)
	return openfga.Tuple{
		Object:   &objectEntity,
		Relation: openfga.Relation(relation),
		Target:   &targetEntity,
	}
}

// TestAddRelation currently verifies the following test cases,
// when new relation control is to be added, please update this comment:
// user -> group
// user -> controller (name)
// user -> controller (uuid)
// user -> model (name)
// user -> model (uuid)
// user -> applicationoffer (name)
// user -> applicationoffer (uuid)
// group -> controller (name)
// group -> controller (uuid)
// group -> model (name)
// group -> model (uuid)
// group -> applicationoffer (name)
// group -> applicationoffer (uuid)
// group#member -> group
func (s *accessControlSuite) TestAddRelation(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	_, err := db.AddGroup(ctx, "test-group2")
	c.Assert(err, gc.IsNil)

	group2 := &dbmodel.GroupEntry{
		Name: "test-group2",
	}
	err = db.GetGroup(ctx, group2)
	c.Assert(err, gc.IsNil)

	c.Assert(err, gc.IsNil)
	type tuple struct {
		object   string
		relation string
		target   string
	}
	type tagTest struct {
		input       tuple
		want        openfga.Tuple
		err         bool
		changesType string
	}

	tagTests := []tagTest{
		// Test user -> controller by name
		{
			input: tuple{"user-" + user.Name, "administrator", "controller-" + controller.Name},
			want: createTuple(
				"user:"+user.Name,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> controller jimm
		{
			input: tuple{"user-" + user.Name, "administrator", "controller-jimm"},
			want: createTuple(
				"user:"+user.Name,
				"administrator",
				"controller:"+s.JIMM.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> controller by UUID
		{
			input: tuple{"user-" + user.Name, "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"user:"+user.Name,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> group
		{
			input: tuple{"user-" + user.Name, "member", "group-" + group.Name},
			want: createTuple(
				"user:"+user.Name,
				"member",
				"group:"+group.UUID,
			),
			err:         false,
			changesType: "group",
		},
		// Test username with dots and @ -> group
		{
			input: tuple{"user-" + "kelvin.lina.test@canonical.com", "member", "group-" + group.Name},
			want: createTuple(
				"user:"+"kelvin.lina.test@canonical.com",
				"member",
				"group:"+group.UUID,
			),
			err:         false,
			changesType: "group",
		},
		// Test group -> controller
		{
			input: tuple{"group-" + "test-group#member", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> model by name
		{
			input: tuple{"user-" + user.Name, "writer", "model-" + user.Name + "/" + model.Name},
			want: createTuple(
				"user:"+user.Name,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> model by UUID
		{
			input: tuple{"user-" + user.Name, "writer", "model-" + model.UUID.String},
			want: createTuple(
				"user:"+user.Name,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> applicationoffer by name
		{
			input: tuple{"user-" + user.Name, "consumer", "applicationoffer-" + offer.URL},
			want: createTuple(
				"user:"+user.Name,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test user -> applicationoffer by UUID
		{
			input: tuple{"user-" + user.Name, "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"user:"+user.Name,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> controller by name
		{
			input: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.Name},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> controller by UUID
		{
			input: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> model by name
		{
			input: tuple{"group-" + group.Name + "#member", "writer", "model-" + user.Name + "/" + model.Name},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> model by UUID
		{
			input: tuple{"group-" + group.Name + "#member", "writer", "model-" + model.UUID.String},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> applicationoffer by name
		{
			input: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + offer.URL},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> applicationoffer by UUID
		{
			input: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> group
		{
			input: tuple{"group-" + group.Name + "#member", "member", "group-" + group2.Name},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"member",
				"group:"+group2.UUID,
			),
			err:         false,
			changesType: "group",
		},
	}

	for i, tc := range tagTests {
		c.Logf("running test %d", i)
		if i != 0 {
			// Needed due to removing original added relations for this test.
			// Without, we cannot add the relations.
			//
			//nolint:errcheck
			s.COFGAClient.RemoveRelation(ctx, tc.want)
		}
		err := client.AddRelation(&apiparams.AddRelationRequest{
			Tuples: []apiparams.RelationshipTuple{
				{
					Object:       tc.input.object,
					Relation:     tc.input.relation,
					TargetObject: tc.input.target,
				},
			},
		})
		if tc.err {
			c.Assert(err, gc.NotNil)
			c.Assert(err, gc.ErrorMatches, tc.want)
		} else {
			c.Assert(err, gc.IsNil)
			changes, err := s.COFGAClient.ReadChanges(ctx, tc.changesType, 99, "")
			c.Assert(err, gc.IsNil)
			key := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
			c.Assert(*key.User, gc.DeepEquals, tc.want.Object.String())
			c.Assert(*key.Relation, gc.DeepEquals, tc.want.Relation.String())
			c.Assert(*key.Object, gc.DeepEquals, tc.want.Target.String())
		}
	}
}

// TestRemoveRelation currently verifies the following test cases,
// similar to the TestAddRelation but instead we add the relations and then
// remove them.
// When new relation control is to be added, please update this comment:
// user -> group
// user -> controller (name)
// user -> controller (uuid)
// user -> model (name)
// user -> model (uuid)
// user -> applicationoffer (name)
// user -> applicationoffer (uuid)
// group -> controller (name)
// group -> controller (uuid)
// group -> model (name)
// group -> model (uuid)
// group -> applicationoffer (name)
// group -> applicationoffer (uuid)
func (s *accessControlSuite) TestRemoveRelation(c *gc.C) {
	ctx := context.Background()

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	type tuple struct {
		user     string
		relation string
		object   string
	}
	type tagTest struct {
		toAdd       openfga.Tuple
		toRemove    tuple
		want        openfga.Tuple
		err         bool
		changesType string
	}

	tagTests := []tagTest{
		// Test user -> controller by name
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "administrator", "controller-" + controller.Name},
			want: createTuple(
				"user:"+user.Name,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> controller by UUID
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"user:"+user.Name,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> group
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "member",
				Target:   ofganames.ConvertTag(group.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "member", "group-" + group.Name},
			want: createTuple(
				"user:"+user.Name,
				"member",
				"group:"+group.UUID,
			),
			err:         false,
			changesType: "group",
		},
		// Test group -> controller
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> model by name
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "writer", "model-" + user.Name + "/" + model.Name},
			want: createTuple(
				"user:"+user.Name,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> model by UUID
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "writer", "model-" + model.UUID.String},
			want: createTuple(
				"user:"+user.Name,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> applicationoffer by name
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "consumer", "applicationoffer-" + offer.URL},
			want: createTuple(
				"user:"+user.Name,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test user -> applicationoffer by UUID
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Name, "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"user:"+user.Name,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> controller by name
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.Name},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> controller by UUID
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> model by name
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "writer", "model-" + user.Name + "/" + model.Name},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> model by UUID
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "writer", "model-" + model.UUID.String},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> applicationoffer by name
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + offer.URL},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> applicationoffer by UUID
		{
			toAdd: openfga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"group:"+group.UUID+"#member",
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
	}

	for i, tc := range tagTests {
		c.Logf("running test %d", i)
		ofgaClient := s.JIMM.OpenFGAClient
		err := ofgaClient.AddRelation(context.Background(), tc.toAdd)
		c.Check(err, gc.IsNil)
		changes, err := s.COFGAClient.ReadChanges(ctx, tc.changesType, 99, "")
		c.Assert(err, gc.IsNil)
		key := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
		c.Assert(*key.User, gc.DeepEquals, tc.want.Object.String())
		c.Assert(*key.Relation, gc.DeepEquals, tc.want.Relation.String())
		c.Assert(*key.Object, gc.DeepEquals, tc.want.Target.String())

		err = client.RemoveRelation(&apiparams.RemoveRelationRequest{
			Tuples: []apiparams.RelationshipTuple{
				{
					Object:       tc.toRemove.user,
					Relation:     tc.toRemove.relation,
					TargetObject: tc.toRemove.object,
				},
			},
		})
		if tc.err {
			c.Assert(err, gc.NotNil)
			c.Assert(err, gc.ErrorMatches, tc.want)
		} else {
			c.Assert(err, gc.IsNil)
			changes, err := s.COFGAClient.ReadChanges(ctx, tc.changesType, 99, "")
			c.Assert(err, gc.IsNil)
			change := changes.GetChanges()[len(changes.GetChanges())-1]
			operation := change.GetOperation()
			c.Assert(string(operation), gc.Equals, "TUPLE_OPERATION_DELETE")
			key := change.GetTupleKey()
			c.Assert(*key.User, gc.DeepEquals, tc.want.Object.String())
			c.Assert(*key.Relation, gc.DeepEquals, tc.want.Relation.String())
			c.Assert(*key.Object, gc.DeepEquals, tc.want.Target.String())
		}
	}
}

func (s *accessControlSuite) TestJAASTag(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, group, controller, model, applicationOffer, cloud, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	serviceAccountId := petname.Generate(2, "-") + "@serviceaccount"
	closeClient()

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Name,
	}, {
		tag:             ofganames.ConvertTag(names.NewServiceAccountTag(serviceAccountId)),
		expectedJAASTag: "serviceaccount-" + serviceAccountId,
	}, {
		tag:             ofganames.ConvertTag(group.ResourceTag()),
		expectedJAASTag: "group-" + group.Name,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.Name,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + user.Name + "/" + model.Name,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + applicationOffer.URL,
	}, {
		tag:           &ofganames.Tag{},
		expectedError: "unexpected tag kind: ",
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}}
	for _, test := range tests {
		t, err := jujuapi.ToJAASTag(db, test.tag, true)
		if test.expectedError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(t, gc.Equals, test.expectedJAASTag)
		}
	}
}

func (s *accessControlSuite) TestJAASTagNoUUIDResolution(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, group, controller, model, applicationOffer, cloud, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	serviceAccountId := petname.Generate(2, "-") + "@serviceaccount"
	closeClient()

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Name,
	}, {
		tag:             ofganames.ConvertTag(names.NewServiceAccountTag(serviceAccountId)),
		expectedJAASTag: "serviceaccount-" + serviceAccountId,
	}, {
		tag:             ofganames.ConvertTag(group.ResourceTag()),
		expectedJAASTag: "group-" + group.UUID,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.UUID,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + model.UUID.String,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + applicationOffer.UUID,
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}, {
		tag:             &ofganames.Tag{},
		expectedJAASTag: "-",
	}}
	for _, test := range tests {
		t, err := jujuapi.ToJAASTag(db, test.tag, false)
		if test.expectedError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(t, gc.Equals, test.expectedJAASTag)
		}
	}
}

func (s *accessControlSuite) TestListRelationshipTuples(c *gc.C) {
	ctx := context.Background()
	user, _, controller, _, applicationOffer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	_, err := client.AddGroup(&apiparams.AddGroupRequest{Name: "yellow"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.AddGroup(&apiparams.AddGroupRequest{Name: "orange"})
	c.Assert(err, jc.ErrorIsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       "group-orange#member",
		Relation:     "member",
		TargetObject: "group-yellow",
	}, {
		Object:       "user-" + user.Name,
		Relation:     "member",
		TargetObject: "group-orange",
	}, {
		Object:       "group-yellow#member",
		Relation:     "administrator",
		TargetObject: "controller-" + controller.Name,
	}, {
		Object:       "group-orange#member",
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + applicationOffer.URL,
	}}

	err = client.AddRelation(&apiparams.AddRelationRequest{Tuples: tuples})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{ResolveUUIDs: true})
	c.Assert(err, jc.ErrorIsNil)
	// first three tuples created during setup test
	c.Assert(response.Tuples[12:], jc.DeepEquals, tuples)
	c.Assert(len(response.Errors), gc.Equals, 0)

	response, err = client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{
		Tuple: apiparams.RelationshipTuple{
			TargetObject: "applicationoffer-" + applicationOffer.URL,
		},
		ResolveUUIDs: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Tuples, jc.DeepEquals, []apiparams.RelationshipTuple{tuples[3]})
	c.Assert(len(response.Errors), gc.Equals, 0)

	// Test error message when a resource is not found
	_, err = client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{
		Tuple: apiparams.RelationshipTuple{
			TargetObject: "applicationoffer-" + "fake-offer",
		},
		ResolveUUIDs: true,
	})
	c.Assert(err, gc.ErrorMatches, "failed to parse tuple target object key applicationoffer-fake-offer: application offer not found.*")
}

func (s *accessControlSuite) TestListRelationshipTuplesNoUUIDResolution(c *gc.C) {
	ctx := context.Background()
	_, _, _, _, applicationOffer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	_, err := client.AddGroup(&apiparams.AddGroupRequest{Name: "orange"})
	c.Assert(err, jc.ErrorIsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       "group-orange#member",
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + applicationOffer.UUID,
	}}

	err = client.AddRelation(&apiparams.AddRelationRequest{Tuples: tuples})
	c.Assert(err, jc.ErrorIsNil)

	groupOrange := dbmodel.GroupEntry{Name: "orange"}
	err = s.JIMM.DB().GetGroup(ctx, &groupOrange)
	c.Assert(err, jc.ErrorIsNil)
	expected := []apiparams.RelationshipTuple{{
		Object:       "group-" + groupOrange.UUID + "#member",
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + applicationOffer.UUID,
	}}
	response, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{
		Tuple: apiparams.RelationshipTuple{
			TargetObject: "applicationoffer-" + applicationOffer.URL,
		},
		ResolveUUIDs: false,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Tuples, jc.DeepEquals, expected)
	c.Assert(len(response.Errors), gc.Equals, 0)
}

func (s *accessControlSuite) TestListRelationshipTuplesAfterDeletingGroup(c *gc.C) {
	ctx := context.Background()
	user, _, controller, _, applicationOffer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	_, err := client.AddGroup(&apiparams.AddGroupRequest{Name: "yellow"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.AddGroup(&apiparams.AddGroupRequest{Name: "orange"})
	c.Assert(err, jc.ErrorIsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       "group-orange#member",
		Relation:     "member",
		TargetObject: "group-yellow",
	}, {
		Object:       "user-" + user.Name,
		Relation:     "member",
		TargetObject: "group-orange",
	}, {
		Object:       "group-yellow#member",
		Relation:     "administrator",
		TargetObject: "controller-" + controller.Name,
	}, {
		Object:       "group-orange#member",
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + applicationOffer.URL,
	}}

	err = client.AddRelation(&apiparams.AddRelationRequest{Tuples: tuples})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RemoveGroup(&apiparams.RemoveGroupRequest{Name: "yellow"})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{ResolveUUIDs: true})
	c.Assert(err, jc.ErrorIsNil)
	// Create a new slice of tuples excluding the ones we expect to be deleted.
	responseTuples := response.Tuples[12:]
	c.Assert(responseTuples, gc.HasLen, 2)

	expectedUserToGroupTuple := tuples[1]
	expectedGroupToOfferTuple := tuples[3]

	// Update the target to the group name
	expectedUserToGroupTuple.TargetObject = "group-orange"
	c.Assert(responseTuples[0], gc.DeepEquals, expectedUserToGroupTuple)
	expectedGroupToOfferTuple.Object = "group-orange#member"
	c.Assert(responseTuples[1], gc.DeepEquals, expectedGroupToOfferTuple)

	c.Assert(len(response.Errors), gc.Equals, 0)
}

func (s *accessControlSuite) TestListRelationshipTuplesWithMissingGroups(c *gc.C) {
	ctx := context.Background()
	_, _, _, _, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	_, err := client.AddGroup(&apiparams.AddGroupRequest{Name: "yellow"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.AddGroup(&apiparams.AddGroupRequest{Name: "orange"})
	c.Assert(err, jc.ErrorIsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       "group-orange#member",
		Relation:     "member",
		TargetObject: "group-yellow",
	}}

	err = client.AddRelation(&apiparams.AddRelationRequest{Tuples: tuples})
	c.Assert(err, jc.ErrorIsNil)

	// Delete a group without going through the API.
	group := &dbmodel.GroupEntry{Name: "yellow"}
	err = s.JIMM.Database.GetGroup(ctx, group)
	c.Assert(err, jc.ErrorIsNil)
	err = s.JIMM.Database.RemoveGroup(ctx, group)
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{ResolveUUIDs: true})
	c.Assert(err, jc.ErrorIsNil)
	tupleWithoutDBEntry := tuples[0]
	tupleWithoutDBEntry.TargetObject = "group:" + group.UUID
	// first three tuples created during setup test
	c.Assert(response.Tuples[12], gc.Equals, tupleWithoutDBEntry)
	c.Assert(response.Errors, gc.DeepEquals, []string{"failed to parse target: failed to fetch group information: " + group.UUID})
}

func (s *accessControlSuite) TestCheckRelationAsNonAdmin(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := api.NewClient(conn)

	userAliceKey := "user-alice@canonical.com"
	userBobKey := "user-bob@canonical.com"

	// Verify Bob checking for Alice's permission fails
	input := apiparams.RelationshipTuple{
		Object:       userAliceKey,
		Relation:     "administrator",
		TargetObject: "controller-jimm",
	}
	req := apiparams.CheckRelationRequest{Tuple: input}
	_, err := client.CheckRelation(&req)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	// Verify Bob can check for his own permission.
	input = apiparams.RelationshipTuple{
		Object:       userBobKey,
		Relation:     "administrator",
		TargetObject: "controller-jimm",
	}
	req = apiparams.CheckRelationRequest{Tuple: input}
	_, err = client.CheckRelation(&req)
	c.Assert(err, gc.IsNil)
}

func (s *accessControlSuite) TestCheckRelationOfferReaderFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, _, _, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some tags (tuples) to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	offerTag := ofganames.ConvertTag(offer.ResourceTag())

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Name
	offerJAASKey := "applicationoffer-" + offer.URL

	// Test direct relation to an applicationoffer from a user of a group via "reader" relation

	userToGroupOfferReader := openfga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToOfferReader := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "reader",
		Target:   offerTag,
	} // Make group members reader of offer via member union

	err := ofgaClient.AddRelation(
		ctx,
		userToGroupOfferReader,
		groupToOfferReader,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input apiparams.RelationshipTuple
		want  bool
	}

	tests := []test{

		// Test user-> reader -> aoffer (due to direct relation from group)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "reader",
				TargetObject: offerJAASKey,
			},
			want: true,
		},
		// Test user -> consumer -> offer (FAILS as there is no union or direct relation to writer)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "consumer",
				TargetObject: offerJAASKey,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{Tuple: tc.input}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationOfferConsumerFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, _, _, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	offerTag := ofganames.ConvertTag(offer.ResourceTag())

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Name
	offerJAASKey := "applicationoffer-" + offer.URL

	// Test direct relation to an applicationoffer from a user of a group via "consumer" relation
	userToGroupMember := openfga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToOfferConsumer := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "consumer",
		Target:   offerTag,
	} // Make group members consumer of offer via member union

	err := ofgaClient.AddRelation(
		ctx,
		userToGroupMember,
		groupToOfferConsumer,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input apiparams.RelationshipTuple
		want  bool
	}

	tests := []test{
		// Test user:dugtrio -> consumer -> applicationoffer:test-offer-2 (due to direct relation from group)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "consumer",
				TargetObject: offerJAASKey,
			},
			want: true,
		},
		// Test user:dugtrio -> reader -> applicationoffer:test-offer-2 (due to direct relation from group and union from consumer to reader)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "reader",
				TargetObject: offerJAASKey,
			},
			want: true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{Tuple: tc.input}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationModelReaderFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, _, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	modelTag := ofganames.ConvertTag(model.ResourceTag())

	// Test direct relation to a model from a user of a group via "writer" relation

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Name
	modelJAASKey := "model-" + user.Name + "/" + model.Name

	// Test direct relation to a model from a user of a group via "reader" relation
	userToGroupMember := openfga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToModelReader := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "reader",
		Target:   modelTag,
	} // Make group members writer of model via member union

	err := ofgaClient.AddRelation(
		ctx,
		userToGroupMember,
		groupToModelReader,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input apiparams.RelationshipTuple
		want  bool
	}

	tests := []test{
		// Test user -> reader -> model (due to direct relation from group)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "reader",
				TargetObject: modelJAASKey,
			},
			want: true,
		},
		// Test user -> writer -> model (FAILS as there is no union or direct relation to writer)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "writer",
				TargetObject: modelJAASKey,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{Tuple: tc.input}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationModelWriterFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, _, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	modelTag := ofganames.ConvertTag(model.ResourceTag())

	// Test direct relation to a model from a user of a group via "writer" relation
	userToGroupMember := openfga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToModelWriter := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "writer",
		Target:   modelTag,
	} // Make group members writer of model via member union

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Name
	modelJAASKey := "model-" + user.Name + "/" + model.Name

	err := ofgaClient.AddRelation(
		ctx,
		userToGroupMember,
		groupToModelWriter,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input apiparams.RelationshipTuple
		want  bool
	}

	tests := []test{
		// Test user-> writer -> model
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "writer",
				TargetObject: modelJAASKey,
			},
			want: true,
		},
		// Test user-> reader -> model(due to union from writer to reader)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "reader",
				TargetObject: modelJAASKey,
			},
			want: true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{Tuple: tc.input}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationControllerAdministratorFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	modelTag := ofganames.ConvertTag(model.ResourceTag())
	controllerTag := ofganames.ConvertTag(controller.ResourceTag())
	offerTag := ofganames.ConvertTag(offer.ResourceTag())

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Name
	groupJAASKey := "group-" + group.Name
	controllerJAASKey := "controller-" + controller.Name
	modelJAASKey := "model-" + user.Name + "/" + model.Name
	offerJAASKey := "applicationoffer-" + offer.URL

	// Test the administrator flow of a group user being related to a controller via administrator relation
	userToGroup := openfga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToControllerAdmin := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   controllerTag,
	} // Make group members administrator of controller via member union

	// NOTE (alesstimec) these two shouldn't really be necessary as they should be automatically
	// created.
	controllerToModelAdmin := openfga.Tuple{
		Object:   controllerTag,
		Relation: "controller",
		Target:   modelTag,
	} // Make controller administrators admins of model via administrator union
	modelToAppOfferAdmin := openfga.Tuple{
		Object:   modelTag,
		Relation: "model",
		Target:   offerTag,
	} // Make controller administrators admin of appoffers via administrator union

	err := ofgaClient.AddRelation(
		ctx,
		userToGroup,
		groupToControllerAdmin,
		controllerToModelAdmin,
		modelToAppOfferAdmin,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input apiparams.RelationshipTuple
		want  bool
	}

	tests := []test{
		// Test user -> member -> group
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "member",
				TargetObject: groupJAASKey,
			},
			want: true,
		},
		// Test user-> member -> controller
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "administrator",
				TargetObject: controllerJAASKey,
			},
			want: true,
		},
		// Test user-> administrator -> model
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "administrator",
				TargetObject: modelJAASKey,
			},
			want: true,
		},
		// Test user -> reader -> model (due to group#member -> controller#admin unioned to model #admin)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "reader",
				TargetObject: modelJAASKey,
			},
			want: true,
		},
		// Test user-> writer -> model (due to group#member -> controller#admin unioned to model #admin)
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "writer",
				TargetObject: modelJAASKey,
			},
			want: true,
		},
		// Test user -> administrator -> offer
		{
			input: apiparams.RelationshipTuple{
				Object:       userJAASKey,
				Relation:     "administrator",
				TargetObject: offerJAASKey,
			},
			want: true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{Tuple: tc.input}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

/*
 None-facade related tests
*/

// createTestControllerEnvironment is a utility function creating the necessary components of adding a:
//   - user
//   - user group
//   - controller
//   - model
//   - application offer
//   - cloud
//   - cloud credential
//
// Into the test database, returning the dbmodels to be utilised for values within tests.
//
// It returns all of the latter, but in addition to those, also:
//   - an api client to make calls to an httptest instance of the server
//   - a closure containing a function to close the connection
//
// TODO(ale8k): Make this an implicit thing on the JIMM suite per test & refactor the current state.
// and make the suite argument an interface of the required calls we use here.
func createTestControllerEnvironment(ctx context.Context, c *gc.C, s *accessControlSuite) (
	dbmodel.Identity,
	dbmodel.GroupEntry,
	dbmodel.Controller,
	dbmodel.Model,
	dbmodel.ApplicationOffer,
	dbmodel.Cloud,
	dbmodel.CloudCredential,
	*api.Client,
	func()) {

	db := s.JIMM.Database
	groupEntry, err := db.AddGroup(ctx, "test-group")
	c.Assert(err, gc.IsNil)
	group := dbmodel.GroupEntry{Name: "test-group"}
	err = db.GetGroup(ctx, &group)
	c.Assert(err, gc.IsNil)
	c.Assert(groupEntry.UUID, gc.Equals, group.UUID)

	u, err := dbmodel.NewIdentity(petname.Generate(2, "-") + "@canonical.com")
	c.Assert(err, gc.IsNil)

	c.Assert(db.DB.Create(u).Error, gc.IsNil)

	cloud := dbmodel.Cloud{
		Name: petname.Generate(2, "-"),
		Type: "aws",
		Regions: []dbmodel.CloudRegion{{
			Name: petname.Generate(2, "-"),
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, gc.IsNil)
	id, _ := uuid.NewRandom()
	controller := dbmodel.Controller{
		Name:        petname.Generate(2, "-"),
		UUID:        id.String(),
		CloudName:   cloud.Name,
		CloudRegion: cloud.Regions[0].Name,
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	cred := dbmodel.CloudCredential{
		Name:              petname.Generate(2, "-"),
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, gc.IsNil)

	model := dbmodel.Model{
		Name: petname.Generate(2, "-"),
		UUID: sql.NullString{
			String: id.String(),
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().UTC().Truncate(time.Millisecond),
				Valid: true,
			},
		},
	}

	err = db.AddModel(ctx, &model)
	c.Assert(err, gc.IsNil)

	offerName := petname.Generate(2, "-")
	offerURL, err := crossmodel.ParseOfferURL(controller.Name + ":" + u.Name + "/" + model.Name + "." + offerName)
	c.Assert(err, gc.IsNil)

	offer := dbmodel.ApplicationOffer{
		UUID:            id.String(),
		Name:            offerName,
		ModelID:         model.ID,
		ApplicationName: petname.Generate(2, "-"),
		URL:             offerURL.String(),
	}
	err = db.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, gc.IsNil)
	c.Assert(len(offer.UUID), gc.Equals, 36)

	conn := s.open(c, nil, "alice")
	client := api.NewClient(conn)

	return *u, group, controller, model, offer, cloud, cred, client, func() {
		conn.Close()
	}
}
