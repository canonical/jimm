// Copyright 2025 Canonical.

package openfga_test

import (
	"context"
	"strconv"
	"testing"

	cofga "github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type openFGATestDeps struct {
	ofgaClient  *openfga.OFGAClient
	cofgaClient *cofga.Client
}

func SetupTest(c *qt.C) openFGATestDeps {
	client, cofgaClient, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	return openFGATestDeps{
		ofgaClient:  client,
		cofgaClient: cofgaClient,
	}
}

func TestWritingTuplesToOFGASucceeds(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()

	uuid1, _ := uuid.NewRandom()
	user1 := names.NewUserTag(uuid1.String())
	tuple1 := openfga.Tuple{
		Object:   ofganames.ConvertTag(user1),
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}

	uuid2, _ := uuid.NewRandom()
	user2 := names.NewUserTag(uuid2.String())
	tuple2 := openfga.Tuple{
		Object:   ofganames.ConvertTag(user2),
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}

	err := s.ofgaClient.AddRelation(ctx, tuple1, tuple2)
	c.Assert(err, qt.IsNil)
	changes, err := s.cofgaClient.ReadChanges(ctx, "group", 99, "")
	c.Assert(err, qt.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(ofganames.ConvertTag(user1).String(), qt.Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(ofganames.ConvertTag(user2).String(), qt.Equals, lastInsertedTuple.GetUser())
}

func TestRemovingTuplesFromOFGASucceeds(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()

	// Create tuples before writing to db
	user1 := ofganames.ConvertTag(names.NewUserTag("bob"))
	tuple1 := openfga.Tuple{
		Object:   user1,
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}

	user2 := ofganames.ConvertTag(names.NewUserTag("alice"))
	tuple2 := openfga.Tuple{
		Object:   user2,
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}

	err := s.ofgaClient.AddRelation(ctx, tuple1, tuple2)
	c.Assert(err, qt.IsNil)

	// Delete after insert should succeed.
	err = s.ofgaClient.RemoveRelation(ctx, tuple1, tuple2)
	c.Assert(err, qt.IsNil)
	changes, err := s.cofgaClient.ReadChanges(ctx, "group", 99, "")
	c.Assert(err, qt.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2]
	secondLastKey := secondToLastInsertedTuple.GetTupleKey()
	c.Assert(user1.String(), qt.Equals, secondLastKey.GetUser())
	c.Assert(string(secondToLastInsertedTuple.GetOperation()), qt.Equals, "TUPLE_OPERATION_DELETE")

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1]
	lastKey := lastInsertedTuple.GetTupleKey()
	c.Assert(user2.String(), qt.Equals, lastKey.GetUser())
	c.Assert(string(lastInsertedTuple.GetOperation()), qt.Equals, "TUPLE_OPERATION_DELETE")
}

func TestCheckRelationSucceeds(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	controllerUUID, _ := uuid.NewRandom()
	controller := names.NewControllerTag(controllerUUID.String())

	user := ofganames.ConvertTag(names.NewUserTag("eve"))
	userToGroup := openfga.Tuple{
		Object:   user,
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}
	groupToController := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(groupUUID), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller),
	}

	err := s.ofgaClient.AddRelation(ctx, userToGroup, groupToController)
	c.Assert(err, qt.IsNil)

	checkTuple := openfga.Tuple{
		Object:   user,
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller),
	}
	allowed, err := s.ofgaClient.CheckRelation(ctx, checkTuple, true)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
}

func TestRemoveTuplesSucceeds(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	groupUUID := uuid.NewString()

	// Note (babakks): OpenFGA only supports a limited number of write operation
	// per request (default is 100). That's why we're testing with a large number
	// of tuples (more than 100) to make sure everything works fine despite the
	// limits.

	// Test a large number of tuples
	for i := range 150 {
		tuple := openfga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("test" + strconv.Itoa(i))),
			Relation: "member",
			Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
		}
		err := s.ofgaClient.AddRelation(context.Background(), tuple)
		c.Assert(err, qt.IsNil)
	}

	checkTuple := openfga.Tuple{
		Target: ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}
	c.Logf("checking for tuple %v\n", checkTuple)
	err := s.ofgaClient.RemoveTuples(context.Background(), checkTuple)
	c.Assert(err, qt.IsNil)
	tuples, ct, err := s.ofgaClient.ReadRelatedObjects(context.Background(), openfga.Tuple{}, 50, "")
	c.Assert(err, qt.IsNil)
	c.Assert(ct, qt.Equals, "")
	c.Assert(len(tuples), qt.Equals, 0)

}

func TestAddControllerModel(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	controller := names.NewControllerTag(controllerUUID.String())
	model := names.NewModelTag(modelUUID.String())

	err = s.ofgaClient.AddControllerModel(context.Background(), controller, model)
	c.Assert(err, qt.IsNil)

	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: "controller",
		Target:   ofganames.ConvertTag(model),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
}

func TestRemoveControllerModel(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	controller := names.NewControllerTag(controllerUUID.String())
	model := names.NewModelTag(modelUUID.String())

	err = s.ofgaClient.AddControllerModel(context.Background(), controller, model)
	c.Assert(err, qt.IsNil)

	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: "controller",
		Target:   ofganames.ConvertTag(model),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	err = s.ofgaClient.RemoveControllerModel(context.Background(), controller, model)
	c.Assert(err, qt.IsNil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)
}

func TestRemoveModel(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	controller := names.NewControllerTag(controllerUUID.String())
	model := names.NewModelTag(modelUUID.String())
	appOffer := names.NewApplicationOfferTag("c7af7fc3-5e40-4de7-98f3-a693a83e0a4f")

	err = s.ofgaClient.AddControllerModel(context.Background(), controller, model)
	c.Assert(err, qt.IsNil)

	err = s.ofgaClient.AddModelApplicationOffer(context.Background(), model, appOffer)
	c.Assert(err, qt.IsNil)

	tuples := []openfga.Tuple{
		{
			Object:   ofganames.ConvertTag(controller),
			Relation: "controller",
			Target:   ofganames.ConvertTag(model),
		},
		{
			Object:   ofganames.ConvertTag(model),
			Relation: "model",
			Target:   ofganames.ConvertTag(appOffer),
		},
	}
	for _, tuple := range tuples {
		allowed, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
		c.Assert(err, qt.IsNil)
		c.Assert(allowed, qt.Equals, true)
	}

	err = s.ofgaClient.RemoveModel(context.Background(), model)
	c.Assert(err, qt.IsNil)

	for _, tuple := range tuples {
		allowed, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
		c.Assert(err, qt.IsNil)
		c.Assert(allowed, qt.Equals, false)
	}
}

func TestAddModelApplicationOffer(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	offerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	model := names.NewModelTag(modelUUID.String())
	offer := names.NewApplicationOfferTag(offerUUID.String())

	err = s.ofgaClient.AddModelApplicationOffer(context.Background(), model, offer)
	c.Assert(err, qt.IsNil)

	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(model),
		Relation: "model",
		Target:   ofganames.ConvertTag(offer),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
}

func TestRemoveApplicationOffer(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	offerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	model := names.NewModelTag(modelUUID.String())
	offer := names.NewApplicationOfferTag(offerUUID.String())

	err = s.ofgaClient.AddModelApplicationOffer(context.Background(), model, offer)
	c.Assert(err, qt.IsNil)

	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(model),
		Relation: "model",
		Target:   ofganames.ConvertTag(offer),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	err = s.ofgaClient.RemoveApplicationOffer(context.Background(), offer)
	c.Assert(err, qt.IsNil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)
}

func TestRemoveRoleWithDirectAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	user1 := ofganames.ConvertTag(names.NewUserTag("user1@canonical.com"))
	role1 := ofganames.ConvertTag(jimmnames.NewRoleTag(uuid.NewString()))

	tuples := []openfga.Tuple{
		{
			Object:   user1,
			Relation: ofganames.AssigneeRelation,
			Target:   role1,
		},
	}

	err := s.ofgaClient.AddRelation(
		context.Background(),
		tuples...,
	)
	c.Assert(err, qt.IsNil)

	err = s.ofgaClient.RemoveRole(ctx, jimmnames.NewRoleTag(role1.ID))
	c.Assert(err, qt.IsNil)

	allowed, err := s.ofgaClient.CheckRelation(
		context.TODO(),
		openfga.Tuple{
			Object:   user1,
			Relation: ofganames.AssigneeRelation,
			Target:   role1,
		},
		false,
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, false)
}

func TestRemoveRoleWithAccessViaGroup(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	user1 := ofganames.ConvertTag(names.NewUserTag("user1@canonical.com"))
	group1 := jimmnames.NewGroupTag(uuid.NewString())
	role1 := ofganames.ConvertTag(jimmnames.NewRoleTag(uuid.NewString()))

	tuples := []openfga.Tuple{
		{
			Object:   user1,
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(group1),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(group1, ofganames.MemberRelation),
			Relation: ofganames.AssigneeRelation,
			Target:   role1,
		},
	}

	err := s.ofgaClient.AddRelation(
		context.Background(),
		tuples...,
	)
	c.Assert(err, qt.IsNil)

	allowed, err := s.ofgaClient.CheckRelation(
		context.TODO(),
		openfga.Tuple{
			Object:   user1,
			Relation: ofganames.AssigneeRelation,
			Target:   role1,
		},
		false,
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, true)

	err = s.ofgaClient.RemoveRole(ctx, jimmnames.NewRoleTag(role1.ID))
	c.Assert(err, qt.IsNil)

	allowed, err = s.ofgaClient.CheckRelation(
		context.TODO(),
		openfga.Tuple{
			Object:   user1,
			Relation: ofganames.AssigneeRelation,
			Target:   role1,
		},
		false,
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, false)
}

func TestRemoveGroup(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	group1 := jimmnames.NewGroupTag(uuid.NewString())
	group2 := jimmnames.NewGroupTag(uuid.NewString())
	alice := names.NewUserTag("alice@canonical.com")
	adam := names.NewUserTag("adam@canonical.com")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(group1),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(group2),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group1, ofganames.MemberRelation),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(group2),
	}}

	err := s.ofgaClient.AddRelation(context.Background(), tuples...)
	c.Assert(err, qt.Equals, nil)

	allowed, err := s.ofgaClient.CheckRelation(
		context.TODO(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(alice),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(group2),
		},
		false,
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, true)

	err = s.ofgaClient.RemoveGroup(context.Background(), group1)
	c.Assert(err, qt.Equals, nil)

	err = s.ofgaClient.RemoveGroup(context.Background(), group1)
	c.Assert(err, qt.Equals, nil)

	allowed, err = s.ofgaClient.CheckRelation(
		context.TODO(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(alice),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(group2),
		},
		false,
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, false)
}

func TestRemoveCloud(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	cloud1 := names.NewCloudTag("cloud-1")

	alice := names.NewUserTag("alice@canonical.com")
	adam := names.NewUserTag("adam@canonical.com")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}}

	err := s.ofgaClient.AddRelation(context.Background(), tuples...)
	c.Assert(err, qt.Equals, nil)

	checks := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}}
	for _, check := range checks {
		allowed, err := s.ofgaClient.CheckRelation(context.TODO(), check, false)
		c.Assert(err, qt.Equals, nil)
		c.Assert(allowed, qt.Equals, true)
	}

	err = s.ofgaClient.RemoveCloud(context.Background(), cloud1)
	c.Assert(err, qt.Equals, nil)

	err = s.ofgaClient.RemoveCloud(context.Background(), cloud1)
	c.Assert(err, qt.Equals, nil)

	for _, check := range checks {
		allowed, err := s.ofgaClient.CheckRelation(context.TODO(), check, false)
		c.Assert(err, qt.Equals, nil)
		c.Assert(allowed, qt.Equals, false)
	}
}

func TestAddCloudController(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	cloud := names.NewCloudTag("cloud-1")
	controller := names.NewControllerTag(uuid.NewString())

	check := openfga.Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(cloud),
	}

	allowed, err := s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, false)

	err = s.ofgaClient.AddCloudController(context.Background(), cloud, controller)
	c.Assert(err, qt.Equals, nil)

	err = s.ofgaClient.AddCloudController(context.Background(), cloud, controller)
	c.Assert(err, qt.Equals, nil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, true)
}

func TestAddController(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	jimm := names.NewControllerTag(uuid.NewString())
	controller := names.NewControllerTag(uuid.NewString())

	check := openfga.Tuple{
		Object:   ofganames.ConvertTag(jimm),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(controller),
	}

	allowed, err := s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, false)

	err = s.ofgaClient.AddController(context.Background(), jimm, controller)
	c.Assert(err, qt.Equals, nil)

	err = s.ofgaClient.AddController(context.Background(), jimm, controller)
	c.Assert(err, qt.Equals, nil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(allowed, qt.Equals, true)
}

func TestListObjectsWithContextualTuples(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := context.TODO()

	modelUUIDs := []string{
		"10000000-0000-0000-0000-000000000000",
		"20000000-0000-0000-0000-000000000000",
		"30000000-0000-0000-0000-000000000000",
	}

	expected := make([]openfga.Tag, len(modelUUIDs))
	for i, v := range modelUUIDs {
		expected[i] = openfga.Tag{
			Kind: "model",
			ID:   v,
		}
	}

	groupUUID := uuid.NewString()

	ids, err := s.ofgaClient.ListObjects(ctx, ofganames.ConvertTag(names.NewUserTag("alice")), "reader", "model", []openfga.Tuple{
		{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.ReaderRelation,
			Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[0])),
		},
		// Reader to model via group
		{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(groupUUID), ofganames.MemberRelation),
			Relation: ofganames.ReaderRelation,
			Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[1])),
		},
		// Reader to model via administrator of controller
		{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(names.NewControllerTag("00000000-0000-0000-0000-000000000000")),
		},
		{
			Object:   ofganames.ConvertTag(names.NewControllerTag("00000000-0000-0000-0000-000000000000")),
			Relation: ofganames.ControllerRelation,
			Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[2])),
		},
	})
	c.Assert(err, qt.Equals, nil)

	c.Assert(cmp.Equal(
		ids,
		expected,
		cmpopts.SortSlices(func(want openfga.Tag, expected openfga.Tag) bool {
			return want.ID < expected.ID
		}),
	), qt.Equals, true)
}

func TestCheckRelationWithContextualTuples(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := context.TODO()

	groupID := uuid.NewString()
	modelID := uuid.NewString()

	err := s.ofgaClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewIdPGroupTag(groupID), ofganames.MemberRelation),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag(modelID)),
	})
	c.Assert(err, qt.IsNil)

	check := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag(modelID)),
	}

	allowed, err := s.ofgaClient.CheckRelation(ctx, check, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsFalse)

	allowed, err = s.ofgaClient.CheckRelation(ctx, check, false, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewIdPGroupTag(groupID)),
	})
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsTrue)
}

func TestListObjectsWithPeristedTuples(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := context.TODO()

	modelUUIDs := []string{
		"10000000-0000-0000-0000-000000000000",
		"20000000-0000-0000-0000-000000000000",
		"30000000-0000-0000-0000-000000000000",
	}

	expected := make([]openfga.Tag, len(modelUUIDs))
	for i, v := range modelUUIDs {
		expected[i] = openfga.Tag{
			Kind: "model",
			ID:   v,
		}
	}

	groupUUID := uuid.NewString()

	c.Assert(s.ofgaClient.AddRelation(ctx,
		[]openfga.Tuple{
			{
				Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
				Relation: ofganames.ReaderRelation,
				Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[0])),
			},
			// Reader to model via group
			{
				Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
				Relation: ofganames.MemberRelation,
				Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
			},
			{
				Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(groupUUID), ofganames.MemberRelation),
				Relation: ofganames.ReaderRelation,
				Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[1])),
			},
			// Reader to model via administrator of controller
			{
				Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
				Relation: ofganames.AdministratorRelation,
				Target:   ofganames.ConvertTag(names.NewControllerTag("00000000-0000-0000-0000-000000000000")),
			},
			{
				Object:   ofganames.ConvertTag(names.NewControllerTag("00000000-0000-0000-0000-000000000000")),
				Relation: ofganames.ControllerRelation,
				Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[2])),
			},
		}...,
	), qt.Equals, nil)

	ids, err := s.ofgaClient.ListObjects(ctx, ofganames.ConvertTag(names.NewUserTag("alice")), "reader", "model", nil)
	c.Assert(err, qt.Equals, nil)
	c.Assert(cmp.Equal(
		ids,
		expected,
		cmpopts.SortSlices(func(want openfga.Tag, expected openfga.Tag) bool {
			return want.ID < expected.ID
		}),
	), qt.Equals, true)
}
