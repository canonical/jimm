package openfga_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	cofga "github.com/canonical/ofga"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/names/v4"
	openfga "github.com/openfga/go-sdk"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
)

type openFGATestSuite struct {
	ofgaClient  *ofga.OFGAClient
	cofgaClient *cofga.Client
}

var _ = gc.Suite(&openFGATestSuite{})

func (s *openFGATestSuite) SetUpTest(c *gc.C) {
	client, cofgaClient, _, err := jimmtest.SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)
	s.cofgaClient = cofgaClient
	s.ofgaClient = client
}

func (s *openFGATestSuite) TestWritingTuplesToOFGASucceeds(c *gc.C) {
	ctx := context.Background()

	groupid := "1"

	uuid1, _ := uuid.NewRandom()
	user1 := names.NewUserTag(uuid1.String())
	key1 := ofga.Tuple{
		Object:   ofganames.ConvertTag(user1),
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}

	uuid2, _ := uuid.NewRandom()
	user2 := names.NewUserTag(uuid2.String())
	key2 := ofga.Tuple{
		Object:   ofganames.ConvertTag(user2),
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}

	err := s.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, err := s.cofgaClient.ReadChanges(ctx, "group", 99, "")
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(ofganames.ConvertTag(user1).String(), gc.Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(ofganames.ConvertTag(user2).String(), gc.Equals, lastInsertedTuple.GetUser())
}

func (suite *openFGATestSuite) TestRemovingTuplesFromOFGASucceeds(c *gc.C) {
	ctx := context.Background()

	groupid := "2"

	//Create tuples before writing to db
	user1 := ofganames.ConvertTag(names.NewUserTag("bob"))
	key1 := ofga.Tuple{
		Object:   user1,
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}

	user2 := ofganames.ConvertTag(names.NewUserTag("alice"))
	key2 := ofga.Tuple{
		Object:   user2,
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}

	//Delete before insert should fail
	err := suite.ofgaClient.RemoveRelation(ctx, key1, key2)
	c.Assert(strings.Contains(err.Error(), "cannot delete a tuple which does not exist"), gc.Equals, true)

	err = suite.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)

	//Delete after insert should succeed.
	err = suite.ofgaClient.RemoveRelation(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, err := suite.cofgaClient.ReadChanges(ctx, "group", 99, "")
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2]
	secondLastKey := secondToLastInsertedTuple.GetTupleKey()
	c.Assert(user1.String(), gc.Equals, secondLastKey.GetUser())
	c.Assert(openfga.DELETE, gc.Equals, secondToLastInsertedTuple.GetOperation())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1]
	lastKey := lastInsertedTuple.GetTupleKey()
	c.Assert(user2.String(), gc.Equals, lastKey.GetUser())
	c.Assert(openfga.DELETE, gc.Equals, lastInsertedTuple.GetOperation())
}

func (s *openFGATestSuite) TestCheckRelationSucceeds(c *gc.C) {
	ctx := context.Background()

	groupid := "3"
	controllerUUID, _ := uuid.NewRandom()
	controller := names.NewControllerTag(controllerUUID.String())

	user := ofganames.ConvertTag(names.NewUserTag("eve"))
	userToGroup := ofga.Tuple{
		Object:   user,
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}
	groupToController := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(groupid), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller),
	}

	err := s.ofgaClient.AddRelations(ctx, userToGroup, groupToController)
	c.Assert(err, gc.IsNil)

	checkTuple := ofga.Tuple{
		Object:   user,
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller),
	}
	allowed, err := s.ofgaClient.CheckRelation(ctx, checkTuple, true)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *openFGATestSuite) TestRemoveTuplesSucceeds(c *gc.C) {
	groupid := "4"

	// Test a large number of tuples
	for i := 0; i < 150; i++ {
		key := ofga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("test" + strconv.Itoa(i))),
			Relation: "member",
			Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
		}
		err := s.ofgaClient.AddRelations(context.Background(), key)
		c.Assert(err, gc.IsNil)
	}

	checkKey := ofga.Tuple{
		Target: ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}
	c.Logf("checking for tuple %v\n", checkKey)
	err := s.ofgaClient.RemoveTuples(context.Background(), checkKey)
	c.Assert(err, gc.IsNil)
	tuples, ct, err := s.ofgaClient.ReadRelatedObjects(context.Background(), ofga.Tuple{}, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(ct, gc.Equals, "")
	c.Assert(len(tuples), gc.Equals, 0)

}

func (s *openFGATestSuite) TestAddControllerModel(c *gc.C) {
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)

	controller := names.NewControllerTag(controllerUUID.String())
	model := names.NewModelTag(modelUUID.String())

	err = s.ofgaClient.AddControllerModel(context.Background(), controller, model)
	c.Assert(err, gc.IsNil)

	key := ofga.Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: "controller",
		Target:   ofganames.ConvertTag(model),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *openFGATestSuite) TestRemoveModel(c *gc.C) {
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)

	controller := names.NewControllerTag(controllerUUID.String())
	model := names.NewModelTag(modelUUID.String())

	err = s.ofgaClient.AddControllerModel(context.Background(), controller, model)
	c.Assert(err, gc.IsNil)

	key := ofga.Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: "controller",
		Target:   ofganames.ConvertTag(model),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)

	err = s.ofgaClient.RemoveModel(context.Background(), model)
	c.Assert(err, gc.IsNil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, false)
}

func (s *openFGATestSuite) TestAddModelApplicationOffer(c *gc.C) {
	offerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)

	model := names.NewModelTag(modelUUID.String())
	offer := names.NewApplicationOfferTag(offerUUID.String())

	err = s.ofgaClient.AddModelApplicationOffer(context.Background(), model, offer)
	c.Assert(err, gc.IsNil)

	key := ofga.Tuple{
		Object:   ofganames.ConvertTag(model),
		Relation: "model",
		Target:   ofganames.ConvertTag(offer),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *openFGATestSuite) TestRemoveApplicationOffer(c *gc.C) {
	offerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)

	model := names.NewModelTag(modelUUID.String())
	offer := names.NewApplicationOfferTag(offerUUID.String())

	err = s.ofgaClient.AddModelApplicationOffer(context.Background(), model, offer)
	c.Assert(err, gc.IsNil)

	key := ofga.Tuple{
		Object:   ofganames.ConvertTag(model),
		Relation: "model",
		Target:   ofganames.ConvertTag(offer),
	}
	allowed, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)

	err = s.ofgaClient.RemoveApplicationOffer(context.Background(), offer)
	c.Assert(err, gc.IsNil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, false)
}

func (s *openFGATestSuite) TestRemoveGroup(c *gc.C) {
	group1 := jimmnames.NewGroupTag("1")
	group2 := jimmnames.NewGroupTag("2")
	alice := names.NewUserTag("alice@external")
	adam := names.NewUserTag("adam@external")

	tuples := []ofga.Tuple{{
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

	err := s.ofgaClient.AddRelations(context.Background(), tuples...)
	c.Assert(err, gc.Equals, nil)

	allowed, err := s.ofgaClient.CheckRelation(
		context.TODO(),
		ofga.Tuple{
			Object:   ofganames.ConvertTag(alice),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(group2),
		},
		false,
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, true)

	err = s.ofgaClient.RemoveGroup(context.Background(), group1)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.RemoveGroup(context.Background(), group1)
	c.Assert(err, gc.Equals, nil)

	allowed, err = s.ofgaClient.CheckRelation(
		context.TODO(),
		ofga.Tuple{
			Object:   ofganames.ConvertTag(alice),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(group2),
		},
		false,
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, false)
}

func (s *openFGATestSuite) TestRemoveCloud(c *gc.C) {
	cloud1 := names.NewCloudTag("cloud-1")

	alice := names.NewUserTag("alice@external")
	adam := names.NewUserTag("adam@external")

	tuples := []ofga.Tuple{{
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(cloud1),
	}}

	err := s.ofgaClient.AddRelations(context.Background(), tuples...)
	c.Assert(err, gc.Equals, nil)

	checks := []ofga.Tuple{{
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
		c.Assert(err, gc.Equals, nil)
		c.Assert(allowed, gc.Equals, true)
	}

	err = s.ofgaClient.RemoveCloud(context.Background(), cloud1)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.RemoveCloud(context.Background(), cloud1)
	c.Assert(err, gc.Equals, nil)

	for _, check := range checks {
		allowed, err := s.ofgaClient.CheckRelation(context.TODO(), check, false)
		c.Assert(err, gc.Equals, nil)
		c.Assert(allowed, gc.Equals, false)
	}
}

func (s *openFGATestSuite) TestAddCloudController(c *gc.C) {
	cloud := names.NewCloudTag("cloud-1")
	controller := names.NewControllerTag(uuid.NewString())

	check := ofga.Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(cloud),
	}

	allowed, err := s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, false)

	err = s.ofgaClient.AddCloudController(context.Background(), cloud, controller)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.AddCloudController(context.Background(), cloud, controller)
	c.Assert(err, gc.Equals, nil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *openFGATestSuite) TestAddController(c *gc.C) {
	jimm := names.NewControllerTag(uuid.NewString())
	controller := names.NewControllerTag(uuid.NewString())

	check := ofga.Tuple{
		Object:   ofganames.ConvertTag(jimm),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(controller),
	}

	allowed, err := s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, false)

	err = s.ofgaClient.AddController(context.Background(), jimm, controller)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.AddController(context.Background(), jimm, controller)
	c.Assert(err, gc.Equals, nil)

	allowed, err = s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *openFGATestSuite) TestListObjectsWithContextualTuples(c *gc.C) {
	ctx := context.TODO()

	modelUUIDs := []string{
		"10000000-0000-0000-0000-000000000000",
		"20000000-0000-0000-0000-000000000000",
		"30000000-0000-0000-0000-000000000000",
	}

	expected := make([]string, len(modelUUIDs))
	for i, v := range modelUUIDs {
		expected[i] = "model:" + v
	}

	ids, err := s.ofgaClient.ListObjects(ctx, "user:alice", "reader", "model", []ofga.Tuple{
		{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.ReaderRelation,
			Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[0])),
		},
		// Reader to model via group
		{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewGroupTag("1")),
		},
		{
			Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag("1"), ofganames.MemberRelation),
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
	c.Assert(err, gc.Equals, nil)

	c.Assert(cmp.Equal(
		ids,
		expected,
		cmpopts.SortSlices(func(want string, expected string) bool {
			return want < expected
		}),
	), gc.Equals, true)
}

func (s *openFGATestSuite) TestListObjectsWithPeristedTuples(c *gc.C) {
	ctx := context.TODO()

	modelUUIDs := []string{
		"10000000-0000-0000-0000-000000000000",
		"20000000-0000-0000-0000-000000000000",
		"30000000-0000-0000-0000-000000000000",
	}

	expected := make([]string, len(modelUUIDs))
	for i, v := range modelUUIDs {
		expected[i] = "model:" + v
	}

	c.Assert(s.ofgaClient.AddRelations(ctx,
		[]ofga.Tuple{
			{
				Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
				Relation: ofganames.ReaderRelation,
				Target:   ofganames.ConvertTag(names.NewModelTag(modelUUIDs[0])),
			},
			// Reader to model via group
			{
				Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
				Relation: ofganames.MemberRelation,
				Target:   ofganames.ConvertTag(jimmnames.NewGroupTag("1")),
			},
			{
				Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag("1"), ofganames.MemberRelation),
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
	), gc.Equals, nil)

	ids, err := s.ofgaClient.ListObjects(ctx, "user:alice", "reader", "model", nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cmp.Equal(
		ids,
		expected,
		cmpopts.SortSlices(func(want string, expected string) bool {
			return want < expected
		}),
	), gc.Equals, true)
}

func Test(t *testing.T) {
	gc.TestingT(t)
}
