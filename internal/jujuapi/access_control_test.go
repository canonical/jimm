package jujuapi_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/google/uuid"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type accessControlSuite struct {
	websocketSuite
}

var _ = gc.Suite(&accessControlSuite{})

func (s *accessControlSuite) TestAddGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)
	err := client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, gc.ErrorMatches, ".*already exists.*")
}

func (s *accessControlSuite) TestAddRelationSucceeds(c *gc.C) {
	ctx := context.Background()
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)

	goodParams := &apiparams.AddRelationRequest{
		Tuples: []apiparams.RelationshipTuple{
			{
				Object:       user1,
				Relation:     "member",
				TargetObject: "group:yolo",
			},
			{
				Object:       user2,
				Relation:     "member",
				TargetObject: "group:digbigletts",
			},
		},
	}

	err := client.AddRelation(goodParams)
	c.Assert(err, gc.IsNil)

	changes, _, err := s.OFGAApi.ReadChanges(ctx).Type_("group").Execute()
	c.Assert(err, gc.IsNil)

	lastChange := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(lastChange.GetUser(), gc.Equals, user2)

	secondToLastChange := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	c.Assert(secondToLastChange.GetUser(), gc.Equals, user1)
}

func (s *accessControlSuite) TestAddRelationFailsWhenObjectDoesNotExistOnAuthorisationModel(c *gc.C) {
	// ctx := context.Background()
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	uuid1, _ := uuid.NewRandom()
	group1 := fmt.Sprintf("group:%s", uuid1)
	badParams := &apiparams.AddRelationRequest{
		Tuples: []apiparams.RelationshipTuple{
			{
				Object:       group1,
				Relation:     "member",
				TargetObject: "missingobjecttype:yolo22",
			},
		},
	}
	err := client.AddRelation(badParams)
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "type_not_found"), gc.Equals, true)
}
