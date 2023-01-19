package openfga_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	gc "gopkg.in/check.v1"

	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
)

type openFGATestSuite struct {
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

var _ = gc.Suite(&openFGATestSuite{})

func (suite *openFGATestSuite) SetUpTest(c *gc.C) {
	api, client := ofga.SetupTestOFGAClient(c)
	suite.ofgaApi = api
	suite.ofgaClient = client
}

func (suite *openFGATestSuite) TestCreateTupleKey(c *gc.C) {
	key := ofga.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	c.Assert("user:diglett", gc.Equals, key.GetUser())
	c.Assert("legendary", gc.Equals, key.GetRelation())
	c.Assert("pokemon:earth", gc.Equals, key.GetObject())
}

func (suite *openFGATestSuite) TestWritingTuplesToOFGADetectsBadObjects(c *gc.C) {
	ctx := context.Background()
	key1 := ofga.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	key2 := ofga.CreateTupleKey("user:diglett", "awesome", "pokemon:earth")
	key3 := ofga.CreateTupleKey("user:dugtrio", "legendary", "pokemon:fire")

	err := suite.ofgaClient.AddRelations(ctx, key1, key2, key3)
	fgaErrCode, _ := openfga.NewErrorCodeFromValue("validation_error")
	serr, ok := err.(openfga.FgaApiValidationError)
	c.Assert(ok, gc.Equals, true)
	c.Assert(400, gc.Equals, serr.ResponseStatusCode())
	c.Assert("POST", gc.Equals, serr.RequestMethod())
	c.Assert("Write", gc.Equals, serr.EndpointCategory())
	c.Assert(*fgaErrCode, gc.Equals, serr.ResponseCode())
}

func (suite *openFGATestSuite) TestWritingTuplesToOFGADetectsSucceeds(c *gc.C) {
	ctx := context.Background()

	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := ofga.CreateTupleKey(user1, "member", "group:pokemon")

	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := ofga.CreateTupleKey(user2, "member", "group:pokemon")

	err := suite.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, _, err := suite.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(user1, gc.Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(user2, gc.Equals, lastInsertedTuple.GetUser())
}

func (suite *openFGATestSuite) TestDeletingTuplesToOFGADetectsSucceeds(c *gc.C) {
	ctx := context.Background()

	//Create tuples before writing to db
	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := ofga.CreateTupleKey(user1, "member", "group:pokemon")
	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := ofga.CreateTupleKey(user2, "member", "group:pokemon")

	//Delete before insert should fail
	err := suite.ofgaClient.DeleteRelations(ctx, key1, key2)
	_, ok := err.(openfga.FgaApiValidationError)
	c.Assert(ok, gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, "cannot delete a tuple which does not exist")

	err = suite.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)

	//Delete after insert should succeed.
	err = suite.ofgaClient.DeleteRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, _, err := suite.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2]
	secondLastKey := secondToLastInsertedTuple.GetTupleKey()
	c.Assert(user1, gc.Equals, secondLastKey.GetUser())
	c.Assert(openfga.DELETE, gc.Equals, secondToLastInsertedTuple.GetOperation())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1]
	lastKey := lastInsertedTuple.GetTupleKey()
	c.Assert(user2, gc.Equals, lastKey.GetUser())
	c.Assert(openfga.DELETE, gc.Equals, lastInsertedTuple.GetOperation())
}

func (suite *openFGATestSuite) TestDeletingAllTuples(c *gc.C) {
	ctx := context.Background()

	//Create tuples before writing to db
	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := ofga.CreateTupleKey(user1, "member", "group:pokemon")
	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := ofga.CreateTupleKey(user2, "member", "group:pokemon")

	err := suite.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)

	err = suite.ofgaClient.DeleteAllTuples(ctx)
	c.Assert(err, gc.IsNil)

}

func Test(t *testing.T) {
	gc.TestingT(t)
}
