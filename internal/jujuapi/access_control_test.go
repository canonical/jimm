package jujuapi_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
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

func (s *accessControlSuite) TestMapJIMMTagToJujuTagHandlesErrors(c *gc.C) {
	db := s.JIMM.Database
	_, err := jujuapi.MapJIMMTagToJujuTag(db, "unknowntag-blabla")
	c.Assert(err, gc.ErrorMatches, "failed to map tag")

	_, err = jujuapi.MapJIMMTagToJujuTag(db, "controller-mycontroller-that-does-not-exist")
	c.Assert(err, gc.ErrorMatches, "controller does not exist")

	_, err = jujuapi.MapJIMMTagToJujuTag(db, "model-mycontroller-that-does-not-exist/mymodel")
	c.Assert(err, gc.ErrorMatches, "could not find controller user in tag")

	_, err = jujuapi.MapJIMMTagToJujuTag(db, "model-mycontroller-that-does-not-exist:alex/")
	c.Assert(err, gc.ErrorMatches, "could not find controller model in tag")
}

func (s *accessControlSuite) TestMapJIMMTagToJujuTagMapsUsers(c *gc.C) {
	db := s.JIMM.Database
	tag, err := jujuapi.MapJIMMTagToJujuTag(db, "user-alex@externally-werly")
	c.Assert(err, gc.IsNil)
	// The username will go through further validation via juju tags
	c.Assert(tag, gc.Equals, "user-alex@externally-werly")
}

func (s *accessControlSuite) TestMapJIMMTagToJujuTagMapsGroups(c *gc.C) {
	db := s.JIMM.Database
	db.AddGroup(context.Background(), "myhandsomegroupofdigletts")
	tag, err := jujuapi.MapJIMMTagToJujuTag(db, "group-myhandsomegroupofdigletts")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "group-myhandsomegroupofdigletts")
}

func (s *accessControlSuite) TestMapJIMMTagToJujuTagMapsControllerUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err := db.AddCloud(context.Background(), &cloud)
	c.Assert(err, gc.IsNil)

	uuid, _ := uuid.NewRandom()
	controller := dbmodel.Controller{
		Name:      "mycontroller",
		UUID:      uuid.String(),
		CloudName: "test-cloud",
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	tag, err := jujuapi.MapJIMMTagToJujuTag(db, "controller-mycontroller")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "controller-"+uuid.String())
}

func (s *accessControlSuite) TestMapJIMMTagToJujuTagMapsModelUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	uuid, _ := uuid.NewRandom()
	user, _, controller, _, model, _ := createTestControllerEnvironment(ctx, uuid.String(), c, db)
	jimmTag := "model-" + controller.Name + ":" + user.Username + "/" + model.Name

	jujuTag, err := jujuapi.MapJIMMTagToJujuTag(db, jimmTag)

	c.Assert(err, gc.IsNil)
	c.Assert(jujuTag, gc.Equals, "model-"+model.UUID.String)
}

func (s *accessControlSuite) TestMapJIMMTagToJujuTagMapsApplicationOffersUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	uuid, _ := uuid.NewRandom()
	user, _, controller, _, model, offer := createTestControllerEnvironment(ctx, uuid.String(), c, db)
	jimmTag := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name

	jujuTag, err := jujuapi.MapJIMMTagToJujuTag(db, jimmTag)

	c.Assert(err, gc.IsNil)
	c.Assert(jujuTag, gc.Equals, "applicationoffer-"+offer.UUID)
}

func (s *accessControlSuite) TestAddRelationSucceedsForUsers(c *gc.C) {
	ctx := context.Background()
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:user-%s@external", uuid1)

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
	c.Assert(lastChange.GetUser(), gc.Equals, fmt.Sprintf("user:%s@external", uuid1))

	// Now assert the user has in fact been created in DB
	u := dbmodel.User{Username: fmt.Sprintf("%s@external", uuid1)}
	err = s.JIMM.Database.GetUserNoCreate(ctx, &u)
	c.Assert(err, gc.IsNil)
}

func (s *accessControlSuite) TestAddRelationSucceedsForModels(c *gc.C) {
	ctx := context.Background()
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:user-%s@external", uuid1)

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
	c.Assert(lastChange.GetUser(), gc.Equals, fmt.Sprintf("user:%s@external", uuid1))

	// Now assert the user has in fact been created in DB
	u := dbmodel.User{Username: fmt.Sprintf("%s@external", uuid1)}
	err = s.JIMM.Database.GetUserNoCreate(ctx, &u)
	c.Assert(err, gc.IsNil)
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
		// Generic relation tests
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

		// Relation propagation tests
		{input: tuple{fmt.Sprintf("group:group-%s#member", getUuid()), "member", "group:legendaries"}, err: false},
		{input: tuple{"group:groupy-woopy-pokemon#member", "member", "group:legendaries"}, want: ".*failed to validate tag for object:.*", err: true},
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

func (s *accessControlSuite) TestRenameGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RenameGroup(&apiparams.RenameGroupRequest{
		Name:    "test-group",
		NewName: "renamed-group",
	})
	c.Assert(err, gc.ErrorMatches, ".*not found.*")

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RenameGroup(&apiparams.RenameGroupRequest{
		Name:    "test-group",
		NewName: "renamed-group",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func createTestControllerEnvironment(ctx context.Context, uuid string, c *gc.C, db db.Database) (dbmodel.User, dbmodel.Cloud, dbmodel.Controller, dbmodel.CloudCredential, dbmodel.Model, dbmodel.ApplicationOffer) {
	u := dbmodel.User{
		Username:         "alice@external" + uuid,
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u).Error, gc.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud" + uuid,
		Type: "test-provider" + uuid,
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1" + uuid,
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, gc.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller-1" + uuid,
		UUID:        uuid,
		CloudName:   "test-cloud" + uuid,
		CloudRegion: "test-region-1" + uuid,
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err := db.AddController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	cred := dbmodel.CloudCredential{
		Name:          "test-credential-1" + uuid,
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, gc.IsNil)

	model := dbmodel.Model{
		Name: "test-model" + uuid,
		UUID: sql.NullString{
			String: uuid,
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().UTC().Truncate(time.Millisecond),
				Valid: true,
			},
		},
	}

	err = db.AddModel(ctx, &model)
	c.Assert(err, gc.IsNil)

	offerURL, err := crossmodel.ParseOfferURL(controller.Name + ":" + u.Username + "/" + model.Name + ".offer1")
	c.Assert(err, gc.IsNil)

	offer := dbmodel.ApplicationOffer{
		UUID:            uuid,
		Name:            "offer1",
		ModelID:         model.ID,
		ApplicationName: "app-1",
		URL:             offerURL.String(),
	}
	err = db.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, gc.IsNil)
	c.Assert(len(offer.UUID), gc.Equals, 36)

	return u, cloud, controller, cred, model, offer
}
