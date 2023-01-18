package jujuapi_test

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	openfga "github.com/openfga/go-sdk"
	. "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
)

type accessControlSuite struct {
	websocketSuite
}

var _ = Suite(&accessControlSuite{})

/*
 Group facade related tests
*/

func (s *accessControlSuite) TestAddGroup(c *C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)
	err := client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, ErrorMatches, ".*already exists.*")
}

func (s *accessControlSuite) TestRemoveGroup(c *C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, ErrorMatches, ".*not found.*")

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessControlSuite) TestRenameGroup(c *C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RenameGroup(&apiparams.RenameGroupRequest{
		Name:    "test-group",
		NewName: "renamed-group",
	})
	c.Assert(err, ErrorMatches, ".*not found.*")

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RenameGroup(&apiparams.RenameGroupRequest{
		Name:    "test-group",
		NewName: "renamed-group",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessControlSuite) TestListGroups(c *C) {
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
	c.Assert(groups, HasLen, 4)
	// groups should be returned in ascending order of name
	c.Assert(groups[0].Name, Equals, "aaaFinalGroup")
	c.Assert(groups[1].Name, Equals, "test-group0")
	c.Assert(groups[2].Name, Equals, "test-group1")
	c.Assert(groups[3].Name, Equals, "test-group2")
}

/*
 Relation facade related tests
*/

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
func (s *accessControlSuite) TestAddRelation(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	db.AddGroup(ctx, "test-group2")
	group2 := &dbmodel.GroupEntry{
		Name: "test-group2",
	}
	err := db.GetGroup(ctx, group2)
	c.Assert(err, IsNil)

	c.Assert(err, IsNil)
	type tuple struct {
		user     string
		relation string
		object   string
	}
	type tagTest struct {
		input       tuple
		want        openfga.TupleKey
		err         bool
		changesType string
	}

	tagTests := []tagTest{
		// Test user -> controller by name
		{
			input: tuple{"user-" + user.Username, "administrator", "controller-" + controller.Name},
			want: ofga.CreateTupleKey(
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
			want: ofga.CreateTupleKey(
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
			want: ofga.CreateTupleKey(
				"user:"+user.Username,
				"member",
				"group:"+strconv.FormatUint(uint64(group.ID), 10),
			),
			err:         false,
			changesType: "group",
		},
		//Test group -> controller
		{
			input: tuple{"group-" + "test-group", "administrator", "controller-" + controller.UUID},
			want: ofga.CreateTupleKey(
				"group:"+strconv.FormatUint(uint64(group.ID), 10),
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		//Test user -> model by name
		{
			input: tuple{"user-" + user.Username, "writer", "model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: ofga.CreateTupleKey(
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
			want: ofga.CreateTupleKey(
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
			want: ofga.CreateTupleKey(
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
			want: ofga.CreateTupleKey(
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
			want: ofga.CreateTupleKey(
				"group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> controller by UUID
		{
			input: tuple{"group-" + group.Name + "#member", "administrator", "controller-" + controller.UUID},
			want: ofga.CreateTupleKey(
				"group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member",
				"administrator",
				"controller:"+controller.UUID,
			),
			err:         false,
			changesType: "controller",
		},
		// Test group -> model by name
		{
			input: tuple{"group-" + group.Name + "#member", "writer", "model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: ofga.CreateTupleKey(
				"group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> model by UUID
		{
			input: tuple{"group-" + group.Name + "#member", "writer", "model-" + model.UUID.String},
			want: ofga.CreateTupleKey(
				"group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member",
				"writer",
				"model:"+model.UUID.String,
			),
			err:         false,
			changesType: "model",
		},
		// Test group -> applicationoffer by name
		{
			input: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name},
			want: ofga.CreateTupleKey(
				"group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member",
				"consumer",
				"applicationoffer:"+offer.UUID,
			),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> applicationoffer by UUID
		{
			input: tuple{"group-" + group.Name + "#member", "consumer", "applicationoffer-" + offer.UUID},
			want: func() openfga.TupleKey {
				return ofga.CreateTupleKey(
					"group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member",
					"consumer",
					"applicationoffer:"+offer.UUID,
				)
			}(),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> group
		{
			input: tuple{"group-" + group.Name + "#member", "member", "group-" + group2.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("member")
				k.SetObject("group:" + strconv.FormatUint(uint64(group2.ID), 10))
				return *k
			}(),
			err:         false,
			changesType: "group",
		},
	}

	for i, tc := range tagTests {
		if i != 0 {
			wr := openfga.NewWriteRequest()
			keys := openfga.NewTupleKeysWithDefaults()
			keys.SetTupleKeys([]openfga.TupleKey{tagTests[i].want})
			wr.SetDeletes(*keys)
			s.OFGAApi.Write(context.Background()).Body(*wr).Execute()
		}
		err := client.AddRelation(&apiparams.AddRelationRequest{
			Tuples: []apiparams.RelationshipTuple{
				{
					Object:       tc.input.user,
					Relation:     tc.input.relation,
					TargetObject: tc.input.object,
				},
			},
		})
		if tc.err {
			c.Assert(err, NotNil)
			c.Assert(err, ErrorMatches, tc.want)
		} else {
			c.Assert(err, IsNil)
			changes, _, err := s.OFGAApi.ReadChanges(ctx).Type_(tc.changesType).Execute()
			c.Assert(err, IsNil)
			key := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
			c.Assert(key, DeepEquals, tc.want)
		}
	}
}

func (s *accessControlSuite) TestJAASTag(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, applicationOffer, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	closeClient()

	tests := []struct {
		tag             string
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             "user:" + user.Username,
		expectedJAASTag: "user-" + user.Username,
	}, {
		tag:             "controller:" + controller.UUID,
		expectedJAASTag: "controller-" + controller.Name,
	}, {
		tag:             "model:" + model.UUID.String,
		expectedJAASTag: "model-" + controller.Name + ":" + user.Username + "/" + model.Name,
	}, {
		tag:             "applicationoffer:" + applicationOffer.UUID,
		expectedJAASTag: "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + applicationOffer.Name,
	}, {
		tag:           "unknown-tag",
		expectedError: "unexpected tag format",
	}}
	for _, test := range tests {
		t, err := jujuapi.ToJAASTag(db, test.tag)
		if test.expectedError != "" {
			c.Assert(err, ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, IsNil)
			c.Assert(t, Equals, test.expectedJAASTag)
		}

	}
}

func (s *accessControlSuite) TestListRelationshipTuples(c *C) {
	ctx := context.Background()
	user, _, controller, model, applicationOffer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	err := client.AddGroup(&apiparams.AddGroupRequest{Name: "yellow"})
	c.Assert(err, jc.ErrorIsNil)
	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "green"})
	c.Assert(err, jc.ErrorIsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       "group-green#member",
		Relation:     "member",
		TargetObject: "group-yellow",
	}, {
		Object:       "user-" + user.Username,
		Relation:     "member",
		TargetObject: "group-green",
	}, {
		Object:       "group-yellow#member",
		Relation:     "administrator",
		TargetObject: "controller-" + controller.Name,
	}, {
		Object:       "group-green#member",
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + applicationOffer.Name,
	}}
	err = client.AddRelation(&apiparams.AddRelationRequest{Tuples: tuples})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Tuples, jc.DeepEquals, tuples)

	response, err = client.ListRelationshipTuples(&apiparams.ListRelationshipTuplesRequest{
		Tuple: apiparams.RelationshipTuple{
			TargetObject: "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + applicationOffer.Name,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Tuples, jc.DeepEquals, []apiparams.RelationshipTuple{tuples[3]})

}

func (s *accessControlSuite) TestCheckRelationOfferReaderFlow(c *C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userKey := "user:" + user.Username
	groupKey := "group:" + strconv.FormatUint(uint64(group.ID), 10)
	offerKey := "applicationoffer:" + offer.UUID

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	offerJAASKey := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	// Test direct relation to an applicationoffer from a user of a group via "reader" relation

	userToGroupOfferReader := ofga.CreateTupleKey(userKey, "member", groupKey)        // Make user member of group
	groupToOfferReader := ofga.CreateTupleKey(groupKey+"#member", "reader", offerKey) // Make group members reader of offer via member union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupOfferReader,
		groupToOfferReader,
	)
	c.Assert(err, IsNil)

	type test struct {
		input openfga.TupleKey
		want  bool
	}

	tests := []test{

		// Test user-> reader -> aoffer (due to direct relation from group)
		{
			input: ofga.CreateTupleKey(userJAASKey, "reader", offerJAASKey),
			want:  true,
		},
		// Test user -> consumer -> offer (FAILS as there is no union or direct relation to writer)
		{
			input: ofga.CreateTupleKey(userJAASKey, "consumer", offerJAASKey),
			want:  false,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       *tc.input.User,
				Relation:     *tc.input.Relation,
				TargetObject: *tc.input.Object,
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, IsNil)
		c.Assert(res.Allowed, Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationOfferConsumerFlow(c *C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userKey := "user:" + user.Username
	groupKey := "group:" + strconv.FormatUint(uint64(group.ID), 10)
	offerKey := "applicationoffer:" + offer.UUID

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	offerJAASKey := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	// Test direct relation to an applicationoffer from a user of a group via "consumer" relation
	userToGroupOfferConsumer := ofga.CreateTupleKey(userKey, "member", groupKey)          // Make user member of group
	groupToOfferConsumer := ofga.CreateTupleKey(groupKey+"#member", "consumer", offerKey) // Make group members consumer of offer via member union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupOfferConsumer,
		groupToOfferConsumer,
	)
	c.Assert(err, IsNil)

	type test struct {
		input openfga.TupleKey
		want  bool
	}

	tests := []test{
		// Test user:dugtrio -> consumer -> applicationoffer:test-offer-2 (due to direct relation from group)
		{
			input: ofga.CreateTupleKey(userJAASKey, "consumer", offerJAASKey),
			want:  true,
		},
		// Test user:dugtrio -> reader -> applicationoffer:test-offer-2 (due to direct relation from group and union from consumer to reader)
		{
			input: ofga.CreateTupleKey(userJAASKey, "reader", offerJAASKey),
			want:  true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       *tc.input.User,
				Relation:     *tc.input.Relation,
				TargetObject: *tc.input.Object,
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, IsNil)
		c.Assert(res.Allowed, Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationModelReaderFlow(c *C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userKey := "user:" + user.Username
	groupKey := "group:" + strconv.FormatUint(uint64(group.ID), 10)
	modelKey := "model:" + model.UUID.String

	// Test direct relation to a model from a user of a group via "writer" relation

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	modelJAASKey := "model-" + controller.Name + ":" + user.Username + "/" + model.Name

	// Test direct relation to a model from a user of a group via "reader" relation
	userToGroupModelReader := ofga.CreateTupleKey(userKey, "member", groupKey)        // Make user member of group
	groupToModelReader := ofga.CreateTupleKey(groupKey+"#member", "reader", modelKey) // Make group members writer of model via member union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupModelReader,
		groupToModelReader,
	)
	c.Assert(err, IsNil)

	type test struct {
		input openfga.TupleKey
		want  bool
	}

	tests := []test{
		// Test user -> reader -> model (due to direct relation from group)
		{
			input: ofga.CreateTupleKey(userJAASKey, "reader", modelJAASKey),
			want:  true,
		},
		// Test user -> writer -> model (FAILS as there is no union or direct relation to writer)
		{
			input: ofga.CreateTupleKey(userJAASKey, "writer", modelJAASKey),
			want:  false,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       *tc.input.User,
				Relation:     *tc.input.Relation,
				TargetObject: *tc.input.Object,
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, IsNil)
		c.Assert(res.Allowed, Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationModelWriterFlow(c *C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, _, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userKey := "user:" + user.Username
	groupKey := "group:" + strconv.FormatUint(uint64(group.ID), 10)
	modelKey := "model:" + model.UUID.String

	// Test direct relation to a model from a user of a group via "writer" relation
	userToGroupModelWriter := ofga.CreateTupleKey(userKey, "member", groupKey)        // Make user member of group
	groupToModelWriter := ofga.CreateTupleKey(groupKey+"#member", "writer", modelKey) // Make group members writer of model via member union

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	modelJAASKey := "model-" + controller.Name + ":" + user.Username + "/" + model.Name

	err := ofgaClient.AddRelations(
		ctx,
		userToGroupModelWriter,
		groupToModelWriter,
	)
	c.Assert(err, IsNil)

	type test struct {
		input openfga.TupleKey
		want  bool
	}

	tests := []test{
		// Test user-> writer -> model
		{
			input: ofga.CreateTupleKey(userJAASKey, "writer", modelJAASKey),
			want:  true,
		},
		// Test user-> reader -> model(due to union from writer to reader)
		{
			input: ofga.CreateTupleKey(userJAASKey, "reader", modelJAASKey),
			want:  true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       *tc.input.User,
				Relation:     *tc.input.Relation,
				TargetObject: *tc.input.Object,
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, IsNil)
		c.Assert(res.Allowed, Equals, tc.want)
	}
}

func (s *accessControlSuite) TestCheckRelationControllerAdministratorFlow(c *C) {
	ctx := context.Background()
	ofgaClient := s.JIMM.OpenFGAClient

	user, group, controller, model, offer, _, _, client, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	// Some keys to assist in the creation of tuples within OpenFGA (such that they can be tested against)
	userKey := "user:" + user.Username
	groupKey := "group:" + strconv.FormatUint(uint64(group.ID), 10)
	controllerKey := "controller:" + controller.UUID
	modelKey := "model:" + model.UUID.String
	offerKey := "applicationoffer:" + offer.UUID

	// JAAS style keys, to be translated and checked against UUIDs/users/groups
	userJAASKey := "user-" + user.Username
	groupJAASKey := "group-" + group.Name
	controllerJAASKey := "controller-" + controller.Name
	modelJAASKey := "model-" + controller.Name + ":" + user.Username + "/" + model.Name
	offerJAASKey := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	// Test the administrator flow of a group user being related to a controller via administrator relation
	userToGroup := ofga.CreateTupleKey(userKey, "member", groupKey)                                             // Make user member of group
	groupToControllerAdmin := ofga.CreateTupleKey(groupKey+"#member", "administrator", controllerKey)           // Make group members administrator of controller via member union
	controllerToModelAdmin := ofga.CreateTupleKey(controllerKey+"#administrator", "administrator", modelKey)    // Make controller administrators admins of model via administrator union
	controllerToAppOfferAdmin := ofga.CreateTupleKey(controllerKey+"#administrator", "administrator", offerKey) // Make controller administrators admin of appoffers via administrator union

	err := ofgaClient.AddRelations(
		ctx,
		userToGroup,
		groupToControllerAdmin,
		controllerToModelAdmin,
		controllerToAppOfferAdmin,
	)
	c.Assert(err, IsNil)

	type test struct {
		input openfga.TupleKey
		want  bool
	}

	tests := []test{
		// Test user -> member -> group
		{
			input: ofga.CreateTupleKey(userJAASKey, "member", groupJAASKey),
			want:  true,
		},
		// Test user-> member -> controller
		{
			input: ofga.CreateTupleKey(userJAASKey, "administrator", controllerJAASKey),
			want:  true,
		},
		// Test user-> administrator -> model
		{
			input: ofga.CreateTupleKey(userJAASKey, "administrator", modelJAASKey),
			want:  true,
		},
		// Test user -> reader -> model (due to group#member -> controller#admin unioned to model #admin)
		{
			input: ofga.CreateTupleKey(userJAASKey, "reader", modelJAASKey),
			want:  true,
		},
		// Test user-> writer -> model (due to group#member -> controller#admin unioned to model #admin)
		{
			input: ofga.CreateTupleKey(userJAASKey, "writer", modelJAASKey),
			want:  true,
		},
		// Test user -> administrator -> offer
		{
			input: ofga.CreateTupleKey(userJAASKey, "administrator", offerJAASKey),
			want:  true,
		},
	}

	for _, tc := range tests {
		req := apiparams.CheckRelationRequest{
			Tuple: apiparams.RelationshipTuple{
				Object:       *tc.input.User,
				Relation:     *tc.input.Relation,
				TargetObject: *tc.input.Object,
			},
		}
		res, err := client.CheckRelation(&req)
		c.Assert(err, IsNil)
		c.Assert(res.Allowed, Equals, tc.want)
	}
}

/*
 None-facade related tests
*/

func (s *accessControlSuite) TestResolveTupleObjectHandlesErrors(c *C) {
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
			want:  "failed to map tag",
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
		_, _, err := jujuapi.ResolveTupleObject(db, tc.input)
		c.Assert(err, ErrorMatches, tc.want)
	}
}

func (s *accessControlSuite) TestResolveTupleObjectMapsUsers(c *C) {
	db := s.JIMM.Database
	tag, specifier, err := jujuapi.ResolveTupleObject(db, "user-alex@externally-werly#member")
	c.Assert(err, IsNil)
	c.Assert(tag, Equals, "user-alex@externally-werly")
	c.Assert(specifier, Equals, "#member")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsGroups(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database
	db.AddGroup(context.Background(), "myhandsomegroupofdigletts")
	group := &dbmodel.GroupEntry{
		Name: "myhandsomegroupofdigletts",
	}
	err := db.GetGroup(ctx, group)
	c.Assert(err, IsNil)
	tag, specifier, err := jujuapi.ResolveTupleObject(db, "group-"+group.Name+"#member")
	c.Assert(err, IsNil)
	c.Assert(tag, Equals, "group-"+strconv.FormatUint(uint64(group.ID), 10))
	c.Assert(specifier, Equals, "#member")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsControllerUUIDs(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err := db.AddCloud(context.Background(), &cloud)
	c.Assert(err, IsNil)

	uuid, _ := uuid.NewRandom()
	controller := dbmodel.Controller{
		Name:      "mycontroller",
		UUID:      uuid.String(),
		CloudName: "test-cloud",
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, IsNil)

	tag, specifier, err := jujuapi.ResolveTupleObject(db, "controller-mycontroller#administrator")
	c.Assert(err, IsNil)
	c.Assert(tag, Equals, "controller-"+uuid.String())
	c.Assert(specifier, Equals, "#administrator")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsModelUUIDs(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, _, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	jimmTag := "model-" + controller.Name + ":" + user.Username + "/" + model.Name + "#administrator"

	jujuTag, specifier, err := jujuapi.ResolveTupleObject(db, jimmTag)

	c.Assert(err, IsNil)
	c.Assert(jujuTag, Equals, "model-"+model.UUID.String)
	c.Assert(specifier, Equals, "#administrator")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsApplicationOffersUUIDs(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, offer, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	closeClient()

	jimmTag := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name + "#administrator"

	jujuTag, specifier, err := jujuapi.ResolveTupleObject(db, jimmTag)

	c.Assert(err, IsNil)
	c.Assert(jujuTag, Equals, "applicationoffer-"+offer.UUID)
	c.Assert(specifier, Equals, "#administrator")
}

func (s *accessControlSuite) TestJujuTagFromTuple(c *C) {
	uuid, _ := uuid.NewRandom()
	tag, err := jujuapi.JujuTagFromTuple("user", "user-ale8k@external")
	c.Assert(err, IsNil)
	c.Assert(tag.Id(), Equals, "ale8k@external")

	tag, err = jujuapi.JujuTagFromTuple("group", "group-1")
	c.Assert(err, IsNil)
	c.Assert(tag.Id(), Equals, "1")

	tag, err = jujuapi.JujuTagFromTuple("controller", "controller-"+uuid.String())
	c.Assert(err, IsNil)
	c.Assert(tag.Id(), Equals, uuid.String())

	tag, err = jujuapi.JujuTagFromTuple("model", "model-"+uuid.String())
	c.Assert(err, IsNil)
	c.Assert(tag.Id(), Equals, uuid.String())

	tag, err = jujuapi.JujuTagFromTuple("applicationoffer", "applicationoffer-"+uuid.String())
	c.Assert(err, IsNil)
	c.Assert(tag.Id(), Equals, uuid.String())
}

func (s *accessControlSuite) TestParseTag(c *C) {
	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, _, _, _, _, closeClient := createTestControllerEnvironment(ctx, c, s)
	defer closeClient()

	jimmTag := "model-" + controller.Name + ":" + user.Username + "/" + model.Name + "#administrator"

	// JIMM tag syntax for models
	tag, specifier, err := jujuapi.ParseTag(ctx, db, jimmTag)
	c.Assert(err, IsNil)
	c.Assert(tag.Kind(), Equals, names.ModelTagKind)
	c.Assert(tag.Id(), Equals, model.UUID.String)
	c.Assert(specifier, Equals, "#administrator")

	jujuTag := "model-" + model.UUID.String + "#administrator"

	// Juju tag syntax for models
	tag, specifier, err = jujuapi.ParseTag(ctx, db, jujuTag)
	c.Assert(err, IsNil)
	c.Assert(tag.Id(), Equals, model.UUID.String)
	c.Assert(tag.Kind(), Equals, names.ModelTagKind)
	c.Assert(specifier, Equals, "#administrator")
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
func createTestControllerEnvironment(ctx context.Context, c *C, s *accessControlSuite) (
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
	c.Assert(err, IsNil)
	group := dbmodel.GroupEntry{Name: "test-group"}
	err = db.GetGroup(ctx, &group)
	c.Assert(err, IsNil)

	u := dbmodel.User{
		Username:         petname.Generate(2, "-") + "@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u).Error, IsNil)

	cloud := dbmodel.Cloud{
		Name: petname.Generate(2, "-"),
		Type: "aws",
		Regions: []dbmodel.CloudRegion{{
			Name: petname.Generate(2, "-"),
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, IsNil)
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
	c.Assert(err, IsNil)

	cred := dbmodel.CloudCredential{
		Name:          petname.Generate(2, "-"),
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, IsNil)

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
	c.Assert(err, IsNil)

	offerName := petname.Generate(2, "-")
	offerURL, err := crossmodel.ParseOfferURL(controller.Name + ":" + u.Username + "/" + model.Name + "." + offerName)
	c.Assert(err, IsNil)

	offer := dbmodel.ApplicationOffer{
		UUID:            id.String(),
		Name:            offerName,
		ModelID:         model.ID,
		ApplicationName: petname.Generate(2, "-"),
		URL:             offerURL.String(),
	}
	err = db.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, IsNil)
	c.Assert(len(offer.UUID), Equals, 36)

	conn := s.open(c, nil, "alice")
	client := api.NewClient(conn)

	return u, group, controller, model, offer, cloud, cred, client, func() {
		conn.Close()
	}
}
