package openfga_test

import (
	"context"
	"fmt"
	"testing"

	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	. "gopkg.in/check.v1"
)

type openFGATestSuite struct {
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

var _ = Suite(&openFGATestSuite{})

func (s *openFGATestSuite) SetUpTest(c *C) {
	api, client := ofga.SetupTestOFGAClient(c)
	s.ofgaApi = api
	s.ofgaClient = client
}

func (s *openFGATestSuite) TestCreateTupleKey(c *C) {
	key := ofga.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	c.Assert("user:diglett", Equals, key.GetUser())
	c.Assert("legendary", Equals, key.GetRelation())
	c.Assert("pokemon:earth", Equals, key.GetObject())
}

func (s *openFGATestSuite) TestWritingTuplesToOFGADetectsBadObjects(c *C) {
	ctx := context.Background()
	key1 := ofga.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	key2 := ofga.CreateTupleKey("user:diglett", "awesome", "pokemon:earth")
	key3 := ofga.CreateTupleKey("user:dugtrio", "legendary", "pokemon:fire")

	err := s.ofgaClient.AddRelations(ctx, key1, key2, key3)
	fgaErrCode, _ := openfga.NewErrorCodeFromValue("validation_error")
	serr, ok := err.(openfga.FgaApiValidationError)
	c.Assert(ok, Equals, true)
	c.Assert(400, Equals, serr.ResponseStatusCode())
	c.Assert("POST", Equals, serr.RequestMethod())
	c.Assert("Write", Equals, serr.EndpointCategory())
	c.Assert(*fgaErrCode, Equals, serr.ResponseCode())
}

func (s *openFGATestSuite) TestWritingTuplesToOFGADetectsSucceeds(c *C) {
	ctx := context.Background()

	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := ofga.CreateTupleKey(user1, "member", "group:pokemon")

	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := ofga.CreateTupleKey(user2, "member", "group:pokemon")

	err := s.ofgaClient.AddRelations(ctx, key1, key2)
	c.Assert(err, IsNil)
	changes, _, err := s.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, IsNil)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(user1, Equals, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(user2, Equals, lastInsertedTuple.GetUser())
}

func (s *openFGATestSuite) TestCheckRelationSucceeds(c *C) {
	ctx := context.Background()

	userToGroup := ofga.CreateTupleKey("user:a-handsome-diglett", "member", "group:1")
	groupToController := ofga.CreateTupleKey("group:1#member", "administrator", "controller:imauuid")

	err := s.ofgaClient.AddRelations(ctx, userToGroup, groupToController)
	c.Assert(err, IsNil)

	checkTuple := ofga.CreateTupleKey("user:a-handsome-diglett", "administrator", "controller:imauuid")
	allowed, resoution, err := s.ofgaClient.CheckRelation(ctx, checkTuple, true)
	c.Assert(err, IsNil)
	c.Assert(allowed, Equals, true)
	c.Assert(resoution, Equals, ".(direct).group:1#member.(direct).")

}

func Test(t *testing.T) {
	TestingT(t)
}
