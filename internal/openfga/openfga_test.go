package openfga_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
)

type openFGATestSuite struct {
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

var _ = gc.Suite(&openFGATestSuite{})

func (s *openFGATestSuite) SetUpTest(c *gc.C) {
	api, client, _ := jimmtest.SetupTestOFGAClient(c)
	s.ofgaApi = api
	s.ofgaClient = client
}

func (s *openFGATestSuite) TestCreateTupleKey(c *gc.C) {
	key := ofga.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	c.Assert("user:diglett", gc.Equals, key.GetUser())
	c.Assert("legendary", gc.Equals, key.GetRelation())
	c.Assert("pokemon:earth", gc.Equals, key.GetObject())
}

func (s *openFGATestSuite) TestWritingTuplesToOFGADetectsBadObjects(c *gc.C) {
	ctx := context.Background()
	key1 := ofga.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	key2 := ofga.CreateTupleKey("user:diglett", "awesome", "pokemon:earth")
	key3 := ofga.CreateTupleKey("user:dugtrio", "legendary", "pokemon:fire")

	err := s.ofgaClient.AddRelations(ctx, key1, key2, key3)
	fgaErrCode, _ := openfga.NewErrorCodeFromValue("validation_error")
	serr, ok := err.(openfga.FgaApiValidationError)
	c.Assert(ok, gc.Equals, true)
	c.Assert(400, gc.Equals, serr.ResponseStatusCode())
	c.Assert("POST", gc.Equals, serr.RequestMethod())
	c.Assert("Write", gc.Equals, serr.EndpointCategory())
	c.Assert(*fgaErrCode, gc.Equals, serr.ResponseCode())
}

func (s *openFGATestSuite) TestWritingTuplesToOFGADetectsSucceeds(c *gc.C) {
	ctx := context.Background()

	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := ofga.CreateTupleKey(user1, "member", "group:pokemon")

	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := ofga.CreateTupleKey(user2, "member", "group:pokemon")

	err := s.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, _, err := s.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(user1, gc.Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(user2, gc.Equals, lastInsertedTuple.GetUser())
}

func (suite *openFGATestSuite) TestRemovingTuplesFromOFGASucceeds(c *gc.C) {
	ctx := context.Background()

	//Create tuples before writing to db
	user1 := "user:bob"
	key1 := ofga.CreateTupleKey(user1, "member", "group:pokemon")
	user2 := "user:alice"
	key2 := ofga.CreateTupleKey(user2, "member", "group:pokemon")

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
	c.Assert(user1, gc.Equals, secondLastKey.GetUser())
	c.Assert(openfga.DELETE, gc.Equals, secondToLastInsertedTuple.GetOperation())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1]
	lastKey := lastInsertedTuple.GetTupleKey()
	c.Assert(user2, gc.Equals, lastKey.GetUser())
	c.Assert(openfga.DELETE, gc.Equals, lastInsertedTuple.GetOperation())
}

func (s *openFGATestSuite) TestCheckRelationSucceeds(c *gc.C) {
	ctx := context.Background()

	userToGroup := ofga.CreateTupleKey("user:a-handsome-diglett", "member", "group:1")
	groupToController := ofga.CreateTupleKey("group:1#member", "administrator", "controller:imauuid")

	err := s.ofgaClient.AddRelations(ctx, userToGroup, groupToController)
	c.Assert(err, gc.IsNil)

	checkTuple := ofga.CreateTupleKey("user:a-handsome-diglett", "administrator", "controller:imauuid")
	allowed, resoution, err := s.ofgaClient.CheckRelation(ctx, checkTuple, true)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
	c.Assert(resoution, gc.Equals, ".(direct).group:1#member.(direct).")
}

func (s *openFGATestSuite) TestRemoveTuplesSucceeds(c *gc.C) {

	// Test a large number of tuples
	for i := 0; i < 150; i++ {
		key := ofga.CreateTupleKey("user:test"+strconv.Itoa(i), "member", "group:1")
		err := s.ofgaClient.AddRelations(context.Background(), key)
		c.Assert(err, gc.IsNil)
	}

	key := ofga.CreateTupleKey("", "", "group:1")
	err := s.ofgaClient.RemoveTuples(context.Background(), key)
	c.Assert(err, gc.IsNil)
	res, err := s.ofgaClient.ReadRelatedObjects(context.Background(), nil, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(res.Keys), gc.Equals, 0)

}

func Test(t *testing.T) {
	gc.TestingT(t)
}
