// Copyright 2023 CanonicalLtd.

package jujuapi_test

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	cofga "github.com/canonical/ofga"
	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
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
	err := client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, gc.ErrorMatches, ".*already exists.*")
}

func (s *accessControlSuite) TestRemoveGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, gc.ErrorMatches, ".*not found.*")

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
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

	db.AddGroup(ctx, "test-group2")
	group2 := &dbmodel.GroupEntry{
		Name: "test-group2",
	}
	err := db.GetGroup(ctx, group2)
	c.Assert(err, gc.IsNil)

	tuples := []ofga.Tuple{
		//This tuple should remain as it has no relation to group2
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

	err = s.JIMM.OpenFGAClient.AddRelations(context.Background(), tuples...)
	c.Assert(err, gc.IsNil)
	//Check user has access to model and controller through group2
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

	//Check user access has been revoked.
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

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
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
		err := client.AddGroup(&apiparams.AddGroupRequest{Name: name})
		c.Assert(err, jc.ErrorIsNil)
	}

	groups, err := client.ListGroups()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groups, gc.HasLen, 4)
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
func createTuple(object, relation, target string) ofga.Tuple {
	objectEntity, _ := cofga.ParseEntity(object)
	targetEntity, _ := cofga.ParseEntity(target)
	return ofga.Tuple{
		Object:   &objectEntity,
		Relation: ofganames.Relation(relation),
		Target:   &targetEntity,
	}
}

func stringGroupID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
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

	db.AddGroup(ctx, "test-group2")
	group2 := &dbmodel.GroupEntry{
		Name: "test-group2",
	}
	err := db.GetGroup(ctx, group2)
	c.Assert(err, gc.IsNil)

	c.Assert(err, gc.IsNil)
	type tuple struct {
		object   string
		relation string
		target   string
	}
	type tagTest struct {
		input       tuple
		want        ofga.Tuple
		err         bool
		changesType string
	}

	tagTests := []tagTest{
		// Test user -> controller by name
		{
			input: tuple{"user-" + user.Username, "administrator", "controller-" + controller.Name},
			want: createTuple(
				"user:"+user.Username,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> controller by UUID
		{
			input: tuple{"user-" + user.Username, "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"user:"+user.Username,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		//Test user -> group
		{
			input: tuple{"user-" + user.Username, "member", "group-" + group.Name},
			want: createTuple(
				"user:"+user.Username,
				"member",
				"group:"+stringGroupID(group.ID),
			),
			err:         false,
			changesType: "group",
		},
		//Test group -> controller
		{
			input: tuple{"group-" + "test-group", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+stringGroupID(group.ID),
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		//Test user -> model by name
		{
			input: tuple{"user-" + user.Username, "writer", "model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: createTuple(
				"user:"+user.Username,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> model by UUID
		{
			input: tuple{"user-" + user.Username, "writer", "model-" + model.UUID.String},
			want: createTuple(
				"user:"+user.Username,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> applicationoffer by name
		{
			input: tuple{"user-" + user.Username, "consumer", "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name},
			want: createTuple(
				"user:"+user.Username,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test user -> applicationoffer by UUID
		{
			input: tuple{"user-" + user.Username, "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"user:"+user.Username,
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
				"group:"+stringGroupID(group.ID)+"#member",
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
				"group:"+stringGroupID(group.ID)+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> model by name
		{
			input: tuple{"group-" + group.Name + "#member", "writer", "model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
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
				"group:"+stringGroupID(group.ID)+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> applicationoffer by name
		{
			input: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
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
				"group:"+stringGroupID(group.ID)+"#member",
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
				"group:"+stringGroupID(group.ID)+"#member",
				"member",
				"group:"+stringGroupID(group2.ID),
			),
			err:         false,
			changesType: "group",
		},
	}

	for i, tc := range tagTests {
		c.Logf("running test %d", i)
		if i != 0 {
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
			c.Assert(changes.ContinuationToken, gc.Equals, "")
			key := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
			c.Assert(key, gc.DeepEquals, tc.want)
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
		toAdd       ofga.Tuple
		toRemove    tuple
		want        ofga.Tuple
		err         bool
		changesType string
	}

	tagTests := []tagTest{
		// Test user -> controller by name
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "administrator", "controller-" + controller.Name},
			want: createTuple(
				"user:"+user.Username,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test user -> controller by UUID
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"user:"+user.Username,
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		//Test user -> group
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "member",
				Target:   ofganames.ConvertTag(group.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "member", "group-" + group.Name},
			want: createTuple(
				"user:"+user.Username,
				"member",
				"group:"+stringGroupID(group.ID),
			),
			err:         false,
			changesType: "group",
		},
		//Test group -> controller
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		//Test user -> model by name
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "writer", "model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: createTuple(
				"user:"+user.Username,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> model by UUID
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "writer", "model-" + model.UUID.String},
			want: createTuple(
				"user:"+user.Username,
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test user -> applicationoffer by name
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "consumer", "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name},
			want: createTuple(
				"user:"+user.Username,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test user -> applicationoffer by UUID
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTag(user.ResourceTag()),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"user-" + user.Username, "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"user:"+user.Username,
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> controller by name
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.Name},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> controller by UUID
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "administrator",
				Target:   ofganames.ConvertTag(controller.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.UUID},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> model by name
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "writer", "model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> model by UUID
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "writer",
				Target:   ofganames.ConvertTag(model.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "writer", "model-" + model.UUID.String},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> applicationoffer by name
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> applicationoffer by UUID
		{
			toAdd: ofga.Tuple{
				Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
				Relation: "consumer",
				Target:   ofganames.ConvertTag(offer.ResourceTag()),
			},
			toRemove: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + offer.UUID},
			want: createTuple(
				"group:"+stringGroupID(group.ID)+"#member",
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
		err := ofgaClient.AddRelations(context.Background(), tc.toAdd)
		c.Check(err, gc.IsNil)
		changes, err := s.COFGAClient.ReadChanges(ctx, tc.changesType, 99, "")
		c.Assert(err, gc.IsNil)
		c.Assert(changes.ContinuationToken, gc.Equals, "")
		key := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
		c.Assert(key, gc.DeepEquals, tc.want)

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
			c.Assert(changes.ContinuationToken, gc.Equals, "")
			change := changes.GetChanges()[len(changes.GetChanges())-1]
			operation := change.GetOperation()
			c.Assert(operation, gc.Equals, "TUPLE_OPERATION_DELETE")
			key := change.GetTupleKey()
			c.Assert(key, gc.DeepEquals, tc.want)
		}
	}
}

func (s *accessControlSuite) TestJAASTag(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, applicationOffer, cloud, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	closeClient()

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Username,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.Name,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + controller.Name + ":" + user.Username + "/" + model.Name,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + applicationOffer.Name,
	}, {
		tag:           &ofganames.Tag{},
		expectedError: "unexpected tag kind: ",
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}}
	for _, test := range tests {
		t, err := jujuapi.ToJAASTag(db, test.tag)
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
	user, _, controller, model, applicationOffer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	err := client.AddGroup(&apiparams.AddGroupRequest{Name: "yellow"})
	c.Assert(err, jc.ErrorIsNil)
	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "orange"})
	c.Assert(err, jc.ErrorIsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       "group-orange#member",
		Relation:     "member",
		TargetObject: "group-yellow",
	}, {
		Object:       "user-" + user.Username,
		Relation:     "member",
		TargetObject: "group-orange",
	}, {
		Object:       "group-yellow#member",
		Relation:     "administrator",
		TargetObject: "controller-" + controller.Name,
	}, {
		Object:       "group-orange#member",
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + applicationOffer.Name,
	}}

	err = client.AddRelation(&apiparams.AddRelationRequest{Tuples: tuples})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{})
	c.Assert(err, jc.ErrorIsNil)
	// first three tuples created during setup test
	c.Assert(response.Tuples[12:], jc.DeepEquals, tuples)

	response, err = client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{
		Tuple: apiparams.RelationshipTuple{
			TargetObject: "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + applicationOffer.Name,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Tuples, jc.DeepEquals, []apiparams.RelationshipTuple{tuples[3]})

}

func (s *accessControlSuite) TestCheckRelationOfferReaderFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	offerTag := ofganames.ConvertTag(offer.ResourceTag())

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	offerJAASKey := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	// Test direct relation to an applicationoffer from a user of a group via "reader" relation

	userToGroupOfferReader := ofga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToOfferReader := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "reader",
		Target:   offerTag,
	} // Make group members reader of offer via member union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupOfferReader,
		groupToOfferReader,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input ofga.Tuple
		want  bool
	}

	tests := []test{

		// Test user-> reader -> aoffer (due to direct relation from group)
		{
			input: createTuple(userJAASKey, "reader", offerJAASKey),
			want:  true,
		},
		// Test user -> consumer -> offer (FAILS as there is no union or direct relation to writer)
		{
			input: createTuple(userJAASKey, "consumer", offerJAASKey),
			want:  false,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       tc.input.Object.String(),
				Relation:     tc.input.Relation.String(),
				TargetObject: tc.input.Target.String(),
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationOfferConsumerFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	offerTag := ofganames.ConvertTag(offer.ResourceTag())

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	offerJAASKey := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	// Test direct relation to an applicationoffer from a user of a group via "consumer" relation
	userToGroupMember := ofga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToOfferConsumer := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "consumer",
		Target:   offerTag,
	} // Make group members consumer of offer via member union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupMember,
		groupToOfferConsumer,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input ofga.Tuple
		want  bool
	}

	tests := []test{
		// Test user:dugtrio -> consumer -> applicationoffer:test-offer-2 (due to direct relation from group)
		{
			input: createTuple(userJAASKey, "consumer", offerJAASKey),
			want:  true,
		},
		// Test user:dugtrio -> reader -> applicationoffer:test-offer-2 (due to direct relation from group and union from consumer to reader)
		{
			input: createTuple(userJAASKey, "reader", offerJAASKey),
			want:  true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       tc.input.Object.String(),
				Relation:     tc.input.Relation.String(),
				TargetObject: tc.input.Target.String(),
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationModelReaderFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	modelTag := ofganames.ConvertTag(model.ResourceTag())

	// Test direct relation to a model from a user of a group via "writer" relation

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	modelJAASKey := "model-" + controller.Name + ":" + user.Username + "/" + model.Name

	// Test direct relation to a model from a user of a group via "reader" relation
	userToGroupMember := ofga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToModelReader := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "reader",
		Target:   modelTag,
	} // Make group members writer of model via member union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupMember,
		groupToModelReader,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input ofga.Tuple
		want  bool
	}

	tests := []test{
		// Test user -> reader -> model (due to direct relation from group)
		{
			input: createTuple(userJAASKey, "reader", modelJAASKey),
			want:  true,
		},
		// Test user -> writer -> model (FAILS as there is no union or direct relation to writer)
		{
			input: createTuple(userJAASKey, "writer", modelJAASKey),
			want:  false,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       tc.input.Object.String(),
				Relation:     tc.input.Relation.String(),
				TargetObject: tc.input.Target.String(),
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationModelWriterFlow(c *gc.C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userTag := ofganames.ConvertTag(user.ResourceTag())
	groupTag := ofganames.ConvertTag(group.ResourceTag())
	modelTag := ofganames.ConvertTag(model.ResourceTag())

	// Test direct relation to a model from a user of a group via "writer" relation
	userToGroupMember := ofga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToModelWriter := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "writer",
		Target:   modelTag,
	} // Make group members writer of model via member union

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	modelJAASKey := "model-" + controller.Name + ":" + user.Username + "/" + model.Name

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupMember,
		groupToModelWriter,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input ofga.Tuple
		want  bool
	}

	tests := []test{
		// Test user-> writer -> model
		{
			input: createTuple(userJAASKey, "writer", modelJAASKey),
			want:  true,
		},
		// Test user-> reader -> model(due to union from writer to reader)
		{
			input: createTuple(userJAASKey, "reader", modelJAASKey),
			want:  true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       tc.input.Object.String(),
				Relation:     tc.input.Relation.String(),
				TargetObject: tc.input.Target.String(),
			},
		}
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
	userJAASKey := "user-" + user.Username
	groupJAASKey := "group-" + group.Name
	controllerJAASKey := "controller-" + controller.Name
	modelJAASKey := "model-" + controller.Name + ":" + user.Username + "/" + model.Name
	offerJAASKey := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	// Test the administrator flow of a group user being related to a controller via administrator relation
	userToGroup := ofga.Tuple{
		Object:   userTag,
		Relation: "member",
		Target:   groupTag,
	} // Make user member of group
	groupToControllerAdmin := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   controllerTag,
	} // Make group members administrator of controller via member union

	// NOTE (alesstimec) these two shouldn't really be necessary as they should be automatically
	// created.
	controllerToModelAdmin := ofga.Tuple{
		Object:   controllerTag,
		Relation: "controller",
		Target:   modelTag,
	} // Make controller administrators admins of model via administrator union
	modelToAppOfferAdmin := ofga.Tuple{
		Object:   modelTag,
		Relation: "model",
		Target:   offerTag,
	} // Make controller administrators admin of appoffers via administrator union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroup,
		groupToControllerAdmin,
		controllerToModelAdmin,
		modelToAppOfferAdmin,
	)
	c.Assert(err, gc.IsNil)

	type test struct {
		input ofga.Tuple
		want  bool
	}

	tests := []test{
		// Test user -> member -> group
		{
			input: createTuple(userJAASKey, "member", groupJAASKey),
			want:  true,
		},
		// Test user-> member -> controller
		{
			input: createTuple(userJAASKey, "administrator", controllerJAASKey),
			want:  true,
		},
		// Test user-> administrator -> model
		{
			input: createTuple(userJAASKey, "administrator", modelJAASKey),
			want:  true,
		},
		// Test user -> reader -> model (due to group#member -> controller#admin unioned to model #admin)
		{
			input: createTuple(userJAASKey, "reader", modelJAASKey),
			want:  true,
		},
		// Test user-> writer -> model (due to group#member -> controller#admin unioned to model #admin)
		{
			input: createTuple(userJAASKey, "writer", modelJAASKey),
			want:  true,
		},
		// Test user -> administrator -> offer
		{
			input: createTuple(userJAASKey, "administrator", offerJAASKey),
			want:  true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       tc.input.Object.String(),
				Relation:     tc.input.Relation.String(),
				TargetObject: tc.input.Target.String(),
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(res.Allowed, gc.Equals, tc.want)
	}
}

/*
 None-facade related tests
*/

func (s *accessControlSuite) TestResolveTupleObjectHandlesErrors(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	_, _, controller, model, offer, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	closeClient()

	type test struct {
		input string
		want  string
	}

	tests := []test{
		// Resolves bad tuple objects in general
		{
			input: "unknowntag-blabla",
			want:  "failed to map tag unknowntag",
		},
		// Resolves bad groups where they do not exist
		{
			input: "group-myspecialpokemon-his-name-is-youguessedit-diglett",
			want:  "group not found",
		},
		// Resolves bad controllers where they do not exist
		{
			input: "controller-mycontroller-that-does-not-exist",
			want:  "controller not found",
		},
		// Resolves bad models where the user cannot be obtained from the JIMM tag
		{
			input: "model-mycontroller-that-does-not-exist/mymodel",
			want:  "model not found",
		},
		// Resolves bad models where it cannot be found on the specified controller
		{
			input: "model-" + controller.Name + ":alex/",
			want:  "model not found",
		},
		// Resolves bad applicationoffers where it cannot be found on the specified controller/model combo
		{
			input: "applicationoffer-" + controller.Name + ":alex/" + model.Name + "." + offer.Name + "fluff",
			want:  "application offer not found",
		},
	}
	for _, tc := range tests {
		_, err := jujuapi.ResolveTag(db, tc.input)
		c.Assert(err, gc.ErrorMatches, tc.want)
	}
}

func (s *accessControlSuite) TestResolveTagObjectMapsUsers(c *gc.C) {
	db := s.JIMM.Database
	tag, err := jujuapi.ResolveTag(db, "user-alex@externally-werly#member")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.DeepEquals, ofganames.ConvertTagWithRelation(names.NewUserTag("alex@externally-werly"), ofganames.MemberRelation))
}

func (s *accessControlSuite) TestResolveTupleObjectMapsGroups(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	db.AddGroup(context.Background(), "myhandsomegroupofdigletts")
	group := &dbmodel.GroupEntry{
		Name: "myhandsomegroupofdigletts",
	}
	err := db.GetGroup(ctx, group)
	c.Assert(err, gc.IsNil)
	tag, err := jujuapi.ResolveTag(db, "group-"+group.Name+"#member")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.DeepEquals, ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag("1"), ofganames.MemberRelation))
}

func (s *accessControlSuite) TestResolveTupleObjectMapsControllerUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err := db.AddCloud(context.Background(), &cloud)
	c.Assert(err, gc.IsNil)

	uuid, _ := uuid.NewRandom()
	controller := dbmodel.Controller{
		Name:      "mycontroller",
		UUID:      uuid.String(),
		CloudName: "test-cloud",
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	tag, err := jujuapi.ResolveTag(db, "controller-mycontroller#administrator")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.DeepEquals, ofganames.ConvertTagWithRelation(names.NewControllerTag(uuid.String()), ofganames.AdministratorRelation))
}

func (s *accessControlSuite) TestResolveTupleObjectMapsModelUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, _, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	jimmTag := "model-" + controller.Name + ":" + user.Username + "/" + model.Name + "#administrator"

	tag, err := jujuapi.ResolveTag(db, jimmTag)
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.DeepEquals, ofganames.ConvertTagWithRelation(names.NewModelTag(model.UUID.String), ofganames.AdministratorRelation))

}

func (s *accessControlSuite) TestResolveTupleObjectMapsApplicationOffersUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, offer, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	closeClient()

	jimmTag := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name + "#administrator"

	jujuTag, err := jujuapi.ResolveTag(db, jimmTag)
	c.Assert(err, gc.IsNil)
	c.Assert(jujuTag, gc.DeepEquals, ofganames.ConvertTagWithRelation(names.NewApplicationOfferTag(offer.UUID), ofganames.AdministratorRelation))
}

func (s *accessControlSuite) TestParseTag(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, _, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	jimmTag := "model-" + controller.Name + ":" + user.Username + "/" + model.Name + "#administrator"

	// JIMM tag syntax for models
	tag, err := jujuapi.ParseTag(ctx, db, jimmTag)
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Kind.String(), gc.Equals, names.ModelTagKind)
	c.Assert(tag.ID, gc.Equals, model.UUID.String)
	c.Assert(tag.Relation.String(), gc.Equals, "administrator")

	jujuTag := "model-" + model.UUID.String + "#administrator"

	// Juju tag syntax for models
	tag, err = jujuapi.ParseTag(ctx, db, jujuTag)
	c.Assert(err, gc.IsNil)
	c.Assert(tag.ID, gc.Equals, model.UUID.String)
	c.Assert(tag.Kind.String(), gc.Equals, names.ModelTagKind)
	c.Assert(tag.Relation.String(), gc.Equals, "administrator")
}

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
	dbmodel.User,
	dbmodel.GroupEntry,
	dbmodel.Controller,
	dbmodel.Model,
	dbmodel.ApplicationOffer,
	dbmodel.Cloud,
	dbmodel.CloudCredential,
	*api.Client,
	func()) {

	db := s.JIMM.Database
	err := db.AddGroup(ctx, "test-group")
	c.Assert(err, gc.IsNil)
	group := dbmodel.GroupEntry{Name: "test-group"}
	err = db.GetGroup(ctx, &group)
	c.Assert(err, gc.IsNil)

	u := dbmodel.User{
		Username:         petname.Generate(2, "-") + "@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u).Error, gc.IsNil)

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
		Name:          petname.Generate(2, "-"),
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, gc.IsNil)

	model := dbmodel.Model{
		Name: petname.Generate(2, "-"),
		UUID: sql.NullString{
			String: id.String(),
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Life:              "alive",
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
	offerURL, err := crossmodel.ParseOfferURL(controller.Name + ":" + u.Username + "/" + model.Name + "." + offerName)
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

	return u, group, controller, model, offer, cloud, cred, client, func() {
		conn.Close()
	}
}
