package openfga_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

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
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

var _ = gc.Suite(&openFGATestSuite{})

func (s *openFGATestSuite) SetUpTest(c *gc.C) {
	api, client, _, err := jimmtest.SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)
	s.ofgaApi = api
	s.ofgaClient = client
}

func (s *openFGATestSuite) TestWritingTuplesToOFGASucceeds(c *gc.C) {
	ctx := context.Background()

	groupid := "1"

	uuid1, _ := uuid.NewRandom()
	user1 := names.NewUserTag(uuid1.String())
	key1 := ofga.Tuple{
		Object:   ofganames.FromTag(user1),
		Relation: "member",
		Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}

	uuid2, _ := uuid.NewRandom()
	user2 := names.NewUserTag(uuid2.String())
	key2 := ofga.Tuple{
		Object:   ofganames.FromTag(user2),
		Relation: "member",
		Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}

	err := s.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, _, err := s.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(ofganames.FromTag(user1).String(), gc.Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(ofganames.FromTag(user2).String(), gc.Equals, lastInsertedTuple.GetUser())
}

func (suite *openFGATestSuite) TestRemovingTuplesFromOFGASucceeds(c *gc.C) {
	ctx := context.Background()

	groupid := "2"

	//Create tuples before writing to db
	user1 := ofganames.FromTag(names.NewUserTag("bob"))
	key1 := ofga.Tuple{
		Object:   user1,
		Relation: "member",
		Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}

	user2 := ofganames.FromTag(names.NewUserTag("alice"))
	key2 := ofga.Tuple{
		Object:   user2,
		Relation: "member",
		Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}

	//Delete before insert should fail
	err := suite.ofgaClient.RemoveRelation(ctx, key1, key2)
	c.Assert(strings.Contains(err.Error(), "cannot delete a tuple which does not exist"), gc.Equals, true)

	err = suite.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)

	//Delete after insert should succeed.
	err = suite.ofgaClient.RemoveRelation(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, _, err := suite.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
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

	user := ofganames.FromTag(names.NewUserTag("eve"))
	userToGroup := ofga.Tuple{
		Object:   user,
		Relation: "member",
		Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}
	groupToController := ofga.Tuple{
		Object:   ofganames.FromTagWithRelation(jimmnames.NewGroupTag(groupid), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   ofganames.FromTag(controller),
	}

	err := s.ofgaClient.AddRelations(ctx, userToGroup, groupToController)
	c.Assert(err, gc.IsNil)

	checkTuple := ofga.Tuple{
		Object:   user,
		Relation: "administrator",
		Target:   ofganames.FromTag(controller),
	}
	allowed, resoution, err := s.ofgaClient.CheckRelation(ctx, checkTuple, true)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
	c.Assert(resoution, gc.Equals, ".union.0(direct).group:"+groupid+"#member.(direct).")
}

func (s *openFGATestSuite) TestRemoveTuplesSucceeds(c *gc.C) {
	groupid := "4"

	// Test a large number of tuples
	for i := 0; i < 150; i++ {
		key := ofga.Tuple{
			Object:   ofganames.FromTag(names.NewUserTag("test" + strconv.Itoa(i))),
			Relation: "member",
			Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
		}
		err := s.ofgaClient.AddRelations(context.Background(), key)
		c.Assert(err, gc.IsNil)
	}

	checkKey := ofga.Tuple{
		Target: ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}
	c.Logf("checking for tuple %v\n", checkKey)
	err := s.ofgaClient.RemoveTuples(context.Background(), &checkKey)
	c.Assert(err, gc.IsNil)
	res, err := s.ofgaClient.ReadRelatedObjects(context.Background(), nil, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(res.Tuples), gc.Equals, 0)

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
		Object:   ofganames.FromTag(controller),
		Relation: "controller",
		Target:   ofganames.FromTag(model),
	}
	allowed, _, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
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
		Object:   ofganames.FromTag(controller),
		Relation: "controller",
		Target:   ofganames.FromTag(model),
	}
	allowed, _, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)

	err = s.ofgaClient.RemoveModel(context.Background(), model)
	c.Assert(err, gc.IsNil)

	allowed, _, err = s.ofgaClient.CheckRelation(context.Background(), key, false)
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
		Object:   ofganames.FromTag(model),
		Relation: "model",
		Target:   ofganames.FromTag(offer),
	}
	allowed, _, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
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
		Object:   ofganames.FromTag(model),
		Relation: "model",
		Target:   ofganames.FromTag(offer),
	}
	allowed, _, err := s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)

	err = s.ofgaClient.RemoveApplicationOffer(context.Background(), offer)
	c.Assert(err, gc.IsNil)

	allowed, _, err = s.ofgaClient.CheckRelation(context.Background(), key, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, false)
}

func (s *openFGATestSuite) TestRemoveGroup(c *gc.C) {
	group1 := jimmnames.NewGroupTag("1")
	group2 := jimmnames.NewGroupTag("2")
	alice := names.NewUserTag("alice@external")
	adam := names.NewUserTag("adam@external")

	tuples := []ofga.Tuple{{
		Object:   ofganames.FromTag(alice),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.FromTag(group1),
	}, {
		Object:   ofganames.FromTag(adam),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.FromTag(group2),
	}, {
		Object:   ofganames.FromTagWithRelation(group1, ofganames.MemberRelation),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.FromTag(group2),
	}}

	err := s.ofgaClient.AddRelations(context.Background(), tuples...)
	c.Assert(err, gc.Equals, nil)

	allowed, _, err := s.ofgaClient.CheckRelation(
		context.TODO(),
		ofga.Tuple{
			Object:   ofganames.FromTag(alice),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.FromTag(group2),
		},
		false,
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, true)

	err = s.ofgaClient.RemoveGroup(context.Background(), group1)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.RemoveGroup(context.Background(), group1)
	c.Assert(err, gc.Equals, nil)

	allowed, _, err = s.ofgaClient.CheckRelation(
		context.TODO(),
		ofga.Tuple{
			Object:   ofganames.FromTag(alice),
			Relation: ofganames.MemberRelation,
			Target:   ofganames.FromTag(group2),
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
		Object:   ofganames.FromTag(alice),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.FromTag(cloud1),
	}, {
		Object:   ofganames.FromTag(adam),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.FromTag(cloud1),
	}}

	err := s.ofgaClient.AddRelations(context.Background(), tuples...)
	c.Assert(err, gc.Equals, nil)

	checks := []ofga.Tuple{{
		Object:   ofganames.FromTag(alice),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.FromTag(cloud1),
	}, {
		Object:   ofganames.FromTag(alice),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.FromTag(cloud1),
	}, {
		Object:   ofganames.FromTag(adam),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.FromTag(cloud1),
	}}
	for _, check := range checks {
		allowed, _, err := s.ofgaClient.CheckRelation(context.TODO(), check, false)
		c.Assert(err, gc.Equals, nil)
		c.Assert(allowed, gc.Equals, true)
	}

	err = s.ofgaClient.RemoveCloud(context.Background(), cloud1)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.RemoveCloud(context.Background(), cloud1)
	c.Assert(err, gc.Equals, nil)

	for _, check := range checks {
		allowed, _, err := s.ofgaClient.CheckRelation(context.TODO(), check, false)
		c.Assert(err, gc.Equals, nil)
		c.Assert(allowed, gc.Equals, false)
	}
}

func (s *openFGATestSuite) TestAddCloudController(c *gc.C) {
	cloud := names.NewCloudTag("cloud-1")
	controller := names.NewControllerTag(uuid.NewString())

	check := ofga.Tuple{
		Object:   ofganames.FromTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.FromTag(cloud),
	}

	allowed, _, err := s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, false)

	err = s.ofgaClient.AddCloudController(context.Background(), cloud, controller)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.AddCloudController(context.Background(), cloud, controller)
	c.Assert(err, gc.Equals, nil)

	allowed, _, err = s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *openFGATestSuite) TestAddController(c *gc.C) {
	jimm := names.NewControllerTag(uuid.NewString())
	controller := names.NewControllerTag(uuid.NewString())

	check := ofga.Tuple{
		Object:   ofganames.FromTag(jimm),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.FromTag(controller),
	}

	allowed, _, err := s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, false)

	err = s.ofgaClient.AddController(context.Background(), jimm, controller)
	c.Assert(err, gc.Equals, nil)

	err = s.ofgaClient.AddController(context.Background(), jimm, controller)
	c.Assert(err, gc.Equals, nil)

	allowed, _, err = s.ofgaClient.CheckRelation(context.Background(), check, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(allowed, gc.Equals, true)
}

func Test(t *testing.T) {
	gc.TestingT(t)
}
