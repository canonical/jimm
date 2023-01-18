package jujuapi_test

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	openfga "github.com/openfga/go-sdk"
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

func (s *accessControlSuite) TestResolveTupleObjectHandlesErrors(c *gc.C) {
	ctx := context.Background()
	uuid, _ := uuid.NewRandom()
	db := s.JIMM.Database
	_, _, controller, _, model, offer := createTestControllerEnvironment(ctx, uuid.String(), c, db)

	type test struct {
		input string
		want  string
	}

	tests := []test{
		// Resolves bad tuple objects in general
		{
			input: "unknowntag-blabla",
			want:  "failed to map tag",
		},
		// Resolves bad groups where they do not exist
		{
			input: "group-myspecialpokemon-his-name-is-youguessedit-diglett",
			want:  "group not found",
		},
		// Resolves bad controllers where they do not exist
		{
			input: "controller-mycontroller-that-does-not-exist",
			want:  "controller not found",
		},
		// Resolves bad models where the user cannot be obtained from the JIMM tag
		{
			input: "model-mycontroller-that-does-not-exist/mymodel",
			want:  "model not found",
		},
		// Resolves bad models where it cannot be found on the specified controller
		{
			input: "model-" + controller.Name + ":alex/",
			want:  "model not found",
		},
		// Resolves bad applicationoffers where it cannot be found on the specified controller/model combo
		{
			input: "applicationoffer-" + controller.Name + ":alex/" + model.Name + "." + offer.Name + "fluff",
			want:  "application offer not found",
		},
	}
	for _, tc := range tests {
		_, _, err := jujuapi.ResolveTupleObject(db, tc.input)
		c.Assert(err, gc.ErrorMatches, tc.want)
	}
}

func (s *accessControlSuite) TestResolveTupleObjectMapsUsers(c *gc.C) {
	db := s.JIMM.Database
	tag, specifier, err := jujuapi.ResolveTupleObject(db, "user-alex@externally-werly#member")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "user-alex@externally-werly")
	c.Assert(specifier, gc.Equals, "#member")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsGroups(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	groupName := "myhandsomegroupofdigletts"
	db.AddGroup(context.Background(), groupName)
	group, err := db.GetGroup(ctx, groupName)
	c.Assert(err, gc.IsNil)
	tag, specifier, err := jujuapi.ResolveTupleObject(db, "group-"+groupName+"#member")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "group-"+strconv.FormatUint(uint64(group.ID), 10))
	c.Assert(specifier, gc.Equals, "#member")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsControllerUUIDs(c *gc.C) {
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

	tag, specifier, err := jujuapi.ResolveTupleObject(db, "controller-mycontroller#administrator")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "controller-"+uuid.String())
	c.Assert(specifier, gc.Equals, "#administrator")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsModelUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	uuid, _ := uuid.NewRandom()
	user, _, controller, _, model, _ := createTestControllerEnvironment(ctx, uuid.String(), c, db)
	jimmTag := "model-" + controller.Name + ":" + user.Username + "/" + model.Name + "#administrator"

	jujuTag, specifier, err := jujuapi.ResolveTupleObject(db, jimmTag)

	c.Assert(err, gc.IsNil)
	c.Assert(jujuTag, gc.Equals, "model-"+model.UUID.String)
	c.Assert(specifier, gc.Equals, "#administrator")
}

func (s *accessControlSuite) TestResolveTupleObjectMapsApplicationOffersUUIDs(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	uuid, _ := uuid.NewRandom()
	user, _, controller, _, model, offer := createTestControllerEnvironment(ctx, uuid.String(), c, db)
	jimmTag := "applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + "." + offer.Name + "#administrator"

	jujuTag, specifier, err := jujuapi.ResolveTupleObject(db, jimmTag)

	c.Assert(err, gc.IsNil)
	c.Assert(jujuTag, gc.Equals, "applicationoffer-"+offer.UUID)
	c.Assert(specifier, gc.Equals, "#administrator")
}

func (s *accessControlSuite) TestJujuTagFromTuple(c *gc.C) {
	uuid, _ := uuid.NewRandom()
	tag, err := jujuapi.JujuTagFromTuple("user", "user-ale8k@external")
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Id(), gc.Equals, "ale8k@external")

	tag, err = jujuapi.JujuTagFromTuple("group", "group-1")
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Id(), gc.Equals, "1")

	tag, err = jujuapi.JujuTagFromTuple("controller", "controller-"+uuid.String())
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Id(), gc.Equals, uuid.String())

	tag, err = jujuapi.JujuTagFromTuple("model", "model-"+uuid.String())
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Id(), gc.Equals, uuid.String())

	tag, err = jujuapi.JujuTagFromTuple("applicationoffer", "applicationoffer-"+uuid.String())
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Id(), gc.Equals, uuid.String())
}

func (s *accessControlSuite) TestParseTag(c *gc.C) {
	ctx := context.Background()
	db := s.JIMM.Database
	uuid, _ := uuid.NewRandom()
	user, _, controller, _, model, _ := createTestControllerEnvironment(ctx, uuid.String(), c, db)
	jimmTag := "model:model-" + controller.Name + ":" + user.Username + "/" + model.Name + "#administrator"

	// JIMM tag syntax for models
	tag, specifier, err := jujuapi.ParseTag(ctx, db, jimmTag)
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Kind(), gc.Equals, names.ModelTagKind)
	c.Assert(tag.Id(), gc.Equals, uuid.String())
	c.Assert(specifier, gc.Equals, "#administrator")

	jujuTag := "model:model-" + uuid.String() + "#administrator"

	// Juju tag syntax for models
	tag, specifier, err = jujuapi.ParseTag(ctx, db, jujuTag)
	c.Assert(err, gc.IsNil)
	c.Assert(tag.Id(), gc.Equals, uuid.String())
	c.Assert(tag.Kind(), gc.Equals, names.ModelTagKind)
	c.Assert(specifier, gc.Equals, "#administrator")
}

// TestAddRelation currently verifies the following test cases,
// when new relation control is to be added, please update this comment:
// user -> group
// user -> controller (name)
// user -> controller (uuid)
// user -> model (name)
// user -> model (uuid)
// user -> applicationoffer (name)
// user -> applicationoffer (uuid)
// group -> controller (name)
// group -> controller (uuid)
// group -> model (name)
// group -> model (uuid)
// group -> applicationoffer (name)
// group -> applicationoffer (uuid)
// group#member -> group
func (s *accessControlSuite) TestAddRelation(c *gc.C) {
	ctx := context.Background()
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	db := s.JIMM.Database

	uuid, _ := uuid.NewRandom()
	user, _, controller, _, model, offer := createTestControllerEnvironment(ctx, uuid.String(), c, db)

	db.AddGroup(ctx, "test-group")
	group, err := db.GetGroup(ctx, "test-group")
	c.Assert(err, gc.IsNil)

	db.AddGroup(ctx, "test-group2")
	group2, err := db.GetGroup(ctx, "test-group2")
	c.Assert(err, gc.IsNil)

	c.Assert(err, gc.IsNil)
	type tuple struct {
		user     string
		relation string
		object   string
	}
	type tagTest struct {
		input       tuple
		want        openfga.TupleKey
		err         bool
		changesType string
	}

	tagTests := []tagTest{
		// Test user -> controller by name
		{
			input: tuple{"user:user-" + user.Username, "administrator", "controller:controller-" + controller.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("administrator")
				k.SetObject("controller:" + controller.UUID)
				return *k
			}(),
			err:         false,
			changesType: "controller",
		},
		// Test user -> controller by UUID
		{
			input: tuple{"user:user-" + user.Username, "administrator", "controller:controller-" + controller.UUID},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("administrator")
				k.SetObject("controller:" + controller.UUID)
				return *k
			}(),
			err:         false,
			changesType: "controller",
		},
		//Test user -> group
		{
			input: tuple{"user:user-" + user.Username, "member", "group:group-" + group.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("member")
				k.SetObject("group:" + strconv.FormatUint(uint64(group.ID), 10))
				return *k
			}(),
			err:         false,
			changesType: "group",
		},
		//Test group -> controller
		{
			input: tuple{"group:group-" + "test-group", "administrator", "controller:controller-" + controller.UUID},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10))
				k.SetRelation("administrator")
				k.SetObject("controller:" + controller.UUID)
				return *k
			}(),
			err:         false,
			changesType: "controller",
		},
		//Test user -> model by name
		{
			input: tuple{"user:user-" + user.Username, "writer", "model:model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("writer")
				k.SetObject("model:" + model.UUID.String)
				return *k
			}(),
			err:         false,
			changesType: "model",
		},
		// Test user -> model by UUID
		{
			input: tuple{"user:user-" + user.Username, "writer", "model:model-" + model.UUID.String},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("writer")
				k.SetObject("model:" + model.UUID.String)
				return *k
			}(),
			err:         false,
			changesType: "model",
		},
		// Test user -> applicationoffer by name
		{
			input: tuple{"user:user-" + user.Username, "consumer", "applicationoffer:applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + ".offer1"},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("consumer")
				k.SetObject("applicationoffer:" + offer.UUID)
				return *k
			}(),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test user -> applicationoffer by UUID
		{
			input: tuple{"user:user-" + user.Username, "consumer", "applicationoffer:applicationoffer-" + offer.UUID},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("user:" + user.Username)
				k.SetRelation("consumer")
				k.SetObject("applicationoffer:" + offer.UUID)
				return *k
			}(),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> controller by name
		{
			input: tuple{"group:group-" + group.Name + "#member", "administrator", "controller:controller-" + controller.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("administrator")
				k.SetObject("controller:" + controller.UUID)
				return *k
			}(),
			err:         false,
			changesType: "controller",
		},
		// Test group -> controller by UUID
		{
			input: tuple{"group:group-" + group.Name + "#member", "administrator", "controller:controller-" + controller.UUID},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("administrator")
				k.SetObject("controller:" + controller.UUID)
				return *k
			}(),
			err:         false,
			changesType: "controller",
		},
		// Test group -> model by name
		{
			input: tuple{"group:group-" + group.Name + "#member", "writer", "model:model-" + controller.Name + ":" + user.Username + "/" + model.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("writer")
				k.SetObject("model:" + model.UUID.String)
				return *k
			}(),
			err:         false,
			changesType: "model",
		},
		// Test group -> model by UUID
		{
			input: tuple{"group:group-" + group.Name + "#member", "writer", "model:model-" + model.UUID.String},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("writer")
				k.SetObject("model:" + model.UUID.String)
				return *k
			}(),
			err:         false,
			changesType: "model",
		},
		// Test group -> applicationoffer by name
		{
			input: tuple{"group:group-" + group.Name + "#member", "consumer", "applicationoffer:applicationoffer-" + controller.Name + ":" + user.Username + "/" + model.Name + ".offer1"},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("consumer")
				k.SetObject("applicationoffer:" + offer.UUID)
				return *k
			}(),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> applicationoffer by UUID
		{
			input: tuple{"group:group-" + group.Name + "#member", "consumer", "applicationoffer:applicationoffer-" + offer.UUID},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("consumer")
				k.SetObject("applicationoffer:" + offer.UUID)
				return *k
			}(),
			err:         false,
			changesType: "applicationoffer",
		},
		// Test group -> group
		{
			input: tuple{"group:group-" + group.Name + "#member", "member", "group:group-" + group2.Name},
			want: func() openfga.TupleKey {
				k := openfga.NewTupleKey()
				k.SetUser("group:" + strconv.FormatUint(uint64(group.ID), 10) + "#member")
				k.SetRelation("member")
				k.SetObject("group:" + strconv.FormatUint(uint64(group2.ID), 10))
				return *k
			}(),
			err:         false,
			changesType: "group",
		},
	}

	for i, tc := range tagTests {
		if i != 0 {
			wr := openfga.NewWriteRequest()
			keys := openfga.NewTupleKeysWithDefaults()
			keys.SetTupleKeys([]openfga.TupleKey{tagTests[i].want})
			wr.SetDeletes(*keys)
			s.OFGAApi.Write(context.Background()).Body(*wr).Execute()
		}
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
			changes, _, err := s.OFGAApi.ReadChanges(ctx).Type_(tc.changesType).Execute()
			c.Assert(err, gc.IsNil)
			key := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
			c.Assert(key, gc.DeepEquals, tc.want)
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

func (s *accessControlSuite) TestRemoveGroup(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, gc.ErrorMatches, ".*not found.*")

	err = client.AddGroup(&apiparams.AddGroupRequest{Name: "test-group"})
	c.Assert(err, jc.ErrorIsNil)

	err = client.RemoveGroup(&apiparams.RemoveGroupRequest{
		Name: "test-group",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessControlSuite) TestListGroups(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)

	groupNames := []string{
		"test-group0",
		"test-group1",
		"test-group2",
		"aaaFinalGroup",
	}

	for _, name := range groupNames {
		err := client.AddGroup(&apiparams.AddGroupRequest{Name: name})
		c.Assert(err, jc.ErrorIsNil)
	}

	groups, err := client.ListGroups()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groups, gc.HasLen, 4)
	// groups should be returned in ascending order of name
	c.Assert(groups[0].Name, gc.Equals, "aaaFinalGroup")
	c.Assert(groups[1].Name, gc.Equals, "test-group0")
	c.Assert(groups[2].Name, gc.Equals, "test-group1")
	c.Assert(groups[3].Name, gc.Equals, "test-group2")
}
