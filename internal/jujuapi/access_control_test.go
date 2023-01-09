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
	user1 := fmt.Sprintf("user:user-%s", uuid1)

	goodParams := &apiparams.AddRelationRequest{
		Tuples: []apiparams.RelationshipTuple{
			{
				Object:       user1,
				Relation:     "administrator",
				TargetObject: "controller:yolo",
			},
		},
	}

	err := client.AddRelation(goodParams)
	c.Assert(err, gc.IsNil)

	changes, _, err := s.OFGAApi.ReadChanges(ctx).Type_("controller").Execute()
	c.Assert(err, gc.IsNil)

	lastChange := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	c.Assert(lastChange.GetUser(), gc.Equals, fmt.Sprintf("user:%s", uuid1))
}

func (s *accessControlSuite) TestAddRelationFailsWhenObjectDoesNotExistOnAuthorisationModel(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	model1 := fmt.Sprintf("model:%s", "model-f47ac10b-58cc-4372-a567-0e02b2c3d479")
	badParams := &apiparams.AddRelationRequest{
		Tuples: []apiparams.RelationshipTuple{
			{
				Object:       model1,
				Relation:     "member",
				TargetObject: "missingobjecttype:yolo22",
			},
		},
	}
	err := client.AddRelation(badParams)
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "type_not_found"), gc.Equals, true)
}

func (s *accessControlSuite) TestAddRelationTagValidation(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	type tuple struct {
		user     string
		relation string
		object   string
	}
	type tagTest struct {
		input tuple
		want  string
		err   bool
	}

	getUuid := func() uuid.UUID {
		uuid, _ := uuid.NewRandom()
		return uuid
	}

	tagTests := []tagTest{
		{input: tuple{"diglett:diglett", "member", "group:yolo22"}, want: ".*failed to validate tag for object:.*", err: true},
		{input: tuple{fmt.Sprintf("user:user-%s", getUuid()), "member", "group:yolo22"}, err: false},
		{input: tuple{fmt.Sprintf("user:user-%s@external", getUuid()), "member", "group:yolo22"}, err: false},
		{input: tuple{"user:userr-alex", "member", "group:yolo22"}, want: ".*failed to validate tag for object:.*", err: true},
		{input: tuple{fmt.Sprintf("model:model-%s", getUuid()), "member", "group:yolo22"}, err: false},
		{input: tuple{fmt.Sprintf("user:user-%s@external", getUuid()), "member", "group:yolo22"}, err: false},
		{input: tuple{"model:modelly-model", "member", "group:yolo22"}, want: ".*failed to validate tag for object:.*", err: true},
		{input: tuple{fmt.Sprintf("controller:controller-%s", getUuid()), "member", "group:yolo22"}, err: false},
		{input: tuple{"model:controlly-wolly", "member", "group:yolo22"}, want: ".*failed to validate tag for object:.*", err: true},
		{input: tuple{fmt.Sprintf("group:group-%s", getUuid()), "member", "group:yolo22"}, err: false},
		{input: tuple{"group:groupy-woopy", "member", "group:yolo22"}, want: ".*failed to validate tag for object:.*", err: true},
	}

	for _, tc := range tagTests {
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
			c.Assert(err, gc.NotNil)
			c.Assert(err, gc.ErrorMatches, tc.want)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}
