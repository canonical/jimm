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

func (s *openFGATestSuite) SetUpTest(c *gc.C) {
	api, client := ofga.SetupTestOFGAClient(c)
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

func Test(t *testing.T) {
	gc.TestingT(t)
}
