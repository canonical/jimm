package openfga_test

import (
	"context"
	"fmt"
	"testing"

	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	gc "gopkg.in/check.v1"
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
	key := suite.ofgaClient.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	c.Assert("user:diglett", gc.Equals, key.GetUser())
	c.Assert("legendary", gc.Equals, key.GetRelation())
	c.Assert("pokemon:earth", gc.Equals, key.GetObject())
}

func (suite *openFGATestSuite) TestWritingTuplesToOFGADetectsBadObjects(c *gc.C) {
	ctx := context.Background()
	key1 := suite.ofgaClient.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	key2 := suite.ofgaClient.CreateTupleKey("user:diglett", "awesome", "pokemon:earth")
	key3 := suite.ofgaClient.CreateTupleKey("user:dugtrio", "legendary", "pokemon:fire")

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
	key1 := suite.ofgaClient.CreateTupleKey(user1, "member", "group:pokemon")

	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := suite.ofgaClient.CreateTupleKey(user2, "member", "group:pokemon")

	err := suite.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, gc.IsNil)
	changes, _, err := suite.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, gc.IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(user1, gc.Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(user2, gc.Equals, lastInsertedTuple.GetUser())
}

func Test(t *testing.T) {
	gc.TestingT(t)
}
