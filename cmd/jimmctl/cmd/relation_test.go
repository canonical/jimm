// Copyright 2024 Canonical.

package cmd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"
	yamlv2 "gopkg.in/yaml.v2"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type relationSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestAddRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	group1 := "testGroup1"
	group2 := "testGroup2"
	type tuple struct {
		user     string
		relation string
		target   string
	}
	tests := []struct {
		testName string
		input    tuple
		err      bool
		message  string
	}{
		{
			testName: "Add Group",
			input: tuple{
				user:     "group-" + group1 + "#member",
				relation: "member",
				target:   "group-" + group2,
			},
			err: false,
		},
		{
			testName: "Add admin relation to controller-jimm",
			input: tuple{
				user:     "group-" + group1 + "#member",
				relation: "administrator",
				target:   "controller-jimm",
			},
			err: false,
		},
		{
			testName: "Invalid Relation",
			input: tuple{
				user:     "group-" + group1 + "#member",
				relation: "admin",
				target:   "group-" + group2,
			},
			err:     true,
			message: "unknown relation",
		},
	}

	_, err := s.JimmCmdSuite.JIMM.Database.AddGroup(context.Background(), group1)
	c.Assert(err, gc.IsNil)
	_, err = s.JimmCmdSuite.JIMM.Database.AddGroup(context.Background(), group2)
	c.Assert(err, gc.IsNil)

	for i, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Log("Test: " + tc.testName)
		if tc.err {
			c.Logf("error message: %s", err.Error())
			c.Assert(strings.Contains(err.Error(), tc.message), gc.Equals, true)
		} else {
			c.Assert(err, gc.IsNil)
			tuples, ct, err := s.JimmCmdSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), openfga.Tuple{}, 50, "")
			c.Assert(err, gc.IsNil)
			c.Assert(ct, gc.Equals, "")
			// NOTE: this is a bad test because it relies on the number of related objects. So all the
			// non-failing test cases must be executed before any of the failing tests - failing tests
			// do not add any tuples therefore the following assertion fails.
			c.Assert(len(tuples), gc.Equals, i+3)
		}
	}

}

func (s *relationSuite) TestMissingParamsAddRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "foo", "bar")
	c.Assert(err, gc.ErrorMatches, "target object not specified")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "foo")
	c.Assert(err, gc.ErrorMatches, "relation not specified")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.ErrorMatches, "object not specified")

}

func (s *relationSuite) TestAddRelationViaFileSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	group1 := "testGroup1"
	group2 := "testGroup2"
	group3 := "testGroup3"

	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), group1)
	c.Assert(err, gc.IsNil)
	_, err = cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), group2)
	c.Assert(err, gc.IsNil)
	_, err = cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), group3)
	c.Assert(err, gc.IsNil)

	file, err := os.CreateTemp(".", "relations.json")
	c.Assert(err, gc.IsNil)
	defer os.Remove(file.Name())
	testRelations := `[{"object":"user-alice","relation":"member","target_object":"group-` + group3 + `"},{"object":"group-` + group2 + `#member","relation":"member","target_object":"group-` + group3 + `"}]`
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	tuples, ct, err := s.JimmCmdSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), openfga.Tuple{}, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(ct, gc.Equals, "")
	c.Assert(len(tuples), gc.Equals, 4)
}

func (s *relationSuite) TestAddRelationRejectsUnauthorisedUsers(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *relationSuite) TestRemoveRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	group1 := "testGroup1"
	group2 := "testGroup2"
	type tuple struct {
		user     string
		relation string
		target   string
	}
	tests := []struct {
		testName string
		input    tuple
		err      bool
		message  string
	}{
		{testName: "Remove Group Relation", input: tuple{user: "group-" + group1 + "#member", relation: "member", target: "group-" + group2}, err: false},
	}

	// Create groups and relation
	_, err := s.JimmCmdSuite.JIMM.Database.AddGroup(context.Background(), group1)
	c.Assert(err, gc.IsNil)
	_, err = s.JimmCmdSuite.JIMM.Database.AddGroup(context.Background(), group2)
	c.Assert(err, gc.IsNil)
	totalKeys := 2
	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Assert(err, gc.IsNil)
		totalKeys++
	}

	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Log("Test: " + tc.testName)
		if tc.err {
			c.Assert(err, gc.ErrorMatches, tc.message)
		} else {
			c.Assert(err, gc.IsNil)
			tuples, ct, err := s.JimmCmdSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), openfga.Tuple{}, 50, "")
			c.Assert(err, gc.IsNil)
			c.Assert(ct, gc.Equals, "")
			totalKeys--
			c.Assert(len(tuples), gc.Equals, totalKeys)
		}
	}
}

func (s *relationSuite) TestRemoveRelationViaFileSuperuser(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "alice")
	group1 := "testGroup1"
	group2 := "testGroup2"
	group3 := "testGroup3"

	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), group1)
	c.Assert(err, gc.IsNil)
	_, err = cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), group2)
	c.Assert(err, gc.IsNil)
	_, err = cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), group3)
	c.Assert(err, gc.IsNil)

	file, err := os.CreateTemp(".", "relations.json")
	c.Assert(err, gc.IsNil)
	defer os.Remove(file.Name())
	testRelations := `[{"object":"group-` + group1 + `#member","relation":"member","target_object":"group-` + group3 + `"},{"object":"group-` + group2 + `#member","relation":"member","target_object":"group-` + group3 + `"}]`
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	tuples, ct, err := s.JimmCmdSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), openfga.Tuple{}, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(ct, gc.Equals, "")
	c.Logf("existing relations %v", tuples)
	// Only two relations exist.
	sort.Slice(tuples, func(i, j int) bool {
		return tuples[i].Object.ID < tuples[j].Object.ID
	})
	c.Assert(tuples, gc.DeepEquals, []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("admin")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewControllerTag(s.Params.ControllerUUID)),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewControllerTag(s.Params.ControllerUUID)),
	}})
}

func (s *relationSuite) TestRemoveRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), "group-testGroup1#member", "member", "group-testGroup2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

type environment struct {
	users             []dbmodel.Identity
	clouds            []dbmodel.Cloud
	credentials       []dbmodel.CloudCredential
	controllers       []dbmodel.Controller
	models            []dbmodel.Model
	applicationOffers []dbmodel.ApplicationOffer
}

func initializeEnvironment(c *gc.C, ctx context.Context, db *db.Database, u dbmodel.Identity) *environment {
	env := environment{}

	u1, err := dbmodel.NewIdentity("eve@canonical.com")
	c.Assert(err, gc.IsNil)
	c.Assert(db.DB.Create(u1).Error, gc.IsNil)

	env.users = []dbmodel.Identity{u, *u1}

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, gc.IsNil)
	env.clouds = []dbmodel.Cloud{cloud}

	controller := dbmodel.Controller{
		Name:          "test-controller-1",
		UUID:          "1fffa2ed-8fd9-49f4-94c0-149288891dd6",
		PublicAddress: "test-public-address",
		CACertificate: "test-ca-cert",
		CloudName:     cloud.Name,
		CloudRegion:   cloud.Regions[0].Name,
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, gc.Equals, nil)
	env.controllers = []dbmodel.Controller{controller}

	cred := dbmodel.CloudCredential{
		Name:              "test-credential-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, gc.Equals, nil)
	env.credentials = []dbmodel.CloudCredential{cred}

	model := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "acdbf3e5-67e1-42a2-a2dc-64505265c030",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
	}
	err = db.AddModel(ctx, &model)
	c.Assert(err, gc.IsNil)
	env.models = []dbmodel.Model{model}

	offer := dbmodel.ApplicationOffer{
		ID:              1,
		UUID:            "436b2264-d8f8-4e24-b16f-dd43c4116528",
		URL:             env.controllers[0].Name + ":" + env.models[0].OwnerIdentityName + "/" + env.models[0].Name + ".testoffer1",
		Name:            "testoffer1",
		ModelID:         model.ID,
		Model:           model,
		ApplicationName: "test-app",
		CharmURL:        "cs:test-app:17",
	}
	err = db.AddApplicationOffer(ctx, &offer)
	c.Assert(err, gc.IsNil)
	env.applicationOffers = []dbmodel.ApplicationOffer{offer}

	return &env
}

func (s *relationSuite) TestListRelations(c *gc.C) {
	env := initializeEnvironment(c, context.Background(), &s.JIMM.Database, *s.AdminUser)
	bClient := s.SetupCLIAccess(c, "alice") // alice is superuser

	relations := []apiparams.RelationshipTuple{{
		Object:       "user-" + env.users[0].Name,
		Relation:     "member",
		TargetObject: "group-group-1",
	}, {
		Object:       "group-group-2#member",
		Relation:     "member",
		TargetObject: "group-group-3",
	}, {
		Object:       "group-group-3#member",
		Relation:     "administrator",
		TargetObject: "controller-" + env.controllers[0].Name,
	}, {
		Object:       "group-group-1#member",
		Relation:     "administrator",
		TargetObject: "model-" + env.models[0].OwnerIdentityName + "/" + env.models[0].Name,
	}, {
		Object:       "user-" + env.users[1].Name,
		Relation:     "administrator",
		TargetObject: "applicationoffer-" + env.applicationOffers[0].URL,
	}, {
		Object:       "user-" + env.users[0].Name,
		Relation:     "administrator",
		TargetObject: "serviceaccount-test@serviceaccount",
	}}

	for i := 0; i < cmd.DefaultPageSize+1; i++ {
		groupName := fmt.Sprintf("group-%d", i)
		_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), groupName)
		c.Assert(err, gc.IsNil)

		relations = append(relations, apiparams.RelationshipTuple{
			Object:       "user-" + env.users[1].Name,
			Relation:     "member",
			TargetObject: "group-" + groupName,
		})
	}

	for _, relation := range relations {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), relation.Object, relation.Relation, relation.TargetObject)
		c.Assert(err, gc.IsNil)
	}

	expectedData := apiparams.ListRelationshipTuplesResponse{Tuples: append(
		[]apiparams.RelationshipTuple{{
			Object:       "user-admin",
			Relation:     "administrator",
			TargetObject: "controller-jimm",
		}, {
			Object:       "user-alice@canonical.com",
			Relation:     "administrator",
			TargetObject: "controller-jimm",
		}},
		relations...,
	)}

	context, err := cmdtesting.RunCommand(c, cmd.NewListRelationsCommandForTesting(s.ClientStore(), bClient), "--format", "tabular")
	c.Assert(err, gc.IsNil)
	var builder strings.Builder
	err = cmd.FormatRelationsTabular(&builder, &expectedData)
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, builder.String())

	expectedJSONData, err := json.Marshal(expectedData)
	c.Assert(err, gc.IsNil)
	context, err = cmdtesting.RunCommand(c, cmd.NewListRelationsCommandForTesting(s.ClientStore(), bClient), "--format", "json")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimRight(cmdtesting.Stdout(context), "\n"), gc.Equals, string(expectedJSONData))

	// Necessary to use yamlv2 to match what Juju does.
	expectedYAMLData, err := yamlv2.Marshal(expectedData)
	c.Assert(err, gc.IsNil)
	context, err = cmdtesting.RunCommand(c, cmd.NewListRelationsCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, string(expectedYAMLData))
}

func (s *relationSuite) TestListRelationsWithError(c *gc.C) {
	env := initializeEnvironment(c, context.Background(), &s.JIMM.Database, *s.AdminUser)
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore(), bClient), "group-1")
	c.Assert(err, gc.IsNil)

	relation := apiparams.RelationshipTuple{
		Object:       "user-" + env.users[0].Name,
		Relation:     "member",
		TargetObject: "group-group-1",
	}
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), relation.Object, relation.Relation, relation.TargetObject)
	c.Assert(err, gc.IsNil)

	ctx := context.Background()
	group := &dbmodel.GroupEntry{Name: "group-1"}
	err = s.JIMM.Database.GetGroup(ctx, group)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.RemoveGroup(ctx, group)
	c.Assert(err, gc.IsNil)

	expectedData := apiparams.ListRelationshipTuplesResponse{
		Tuples: []apiparams.RelationshipTuple{{
			Object:       "user-admin",
			Relation:     "administrator",
			TargetObject: "controller-jimm",
		}, {
			Object:       "user-alice@canonical.com",
			Relation:     "administrator",
			TargetObject: "controller-jimm",
		}, {
			Object:       "user-" + env.users[0].Name,
			Relation:     "member",
			TargetObject: "group:" + group.UUID,
		}},
		Errors: []string{
			"failed to parse target: failed to fetch group information: " + group.UUID,
		},
	}
	expectedJSONData, err := json.Marshal(expectedData)
	c.Assert(err, gc.IsNil)
	// Necessary to use yamlv2 to match what Juju does.
	expectedYAMLData, err := yamlv2.Marshal(expectedData)
	c.Assert(err, gc.IsNil)

	context, err := cmdtesting.RunCommand(c, cmd.NewListRelationsCommandForTesting(s.ClientStore(), bClient), "--format", "json")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimRight(cmdtesting.Stdout(context), "\n"), gc.Equals, string(expectedJSONData))

	context, err = cmdtesting.RunCommand(c, cmd.NewListRelationsCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, string(expectedYAMLData))

	context, err = cmdtesting.RunCommand(c, cmd.NewListRelationsCommandForTesting(s.ClientStore(), bClient), "--format", "tabular")
	c.Assert(err, gc.IsNil)
	var builder strings.Builder
	err = cmd.FormatRelationsTabular(&builder, &expectedData)
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, builder.String())
}

// TODO: remove boilerplate of env setup and use initialiseEnvironment
func (s *relationSuite) TestCheckRelationViaSuperuser(c *gc.C) {
	ctx := context.TODO()
	bClient := s.SetupCLIAccess(c, "alice")
	ofgaClient := s.JIMM.OpenFGAClient

	// Add some resources to check against
	db := s.JIMM.Database
	_, err := db.AddGroup(ctx, "test-group")
	c.Assert(err, gc.IsNil)
	group := dbmodel.GroupEntry{Name: "test-group"}
	err = db.GetGroup(ctx, &group)
	c.Assert(err, gc.IsNil)

	u, err := dbmodel.NewIdentity(petname.Generate(2, "-") + "@canonical.com")
	c.Assert(err, gc.IsNil)
	c.Assert(db.DB.Create(u).Error, gc.IsNil)

	cloud := dbmodel.Cloud{
		Name: petname.Generate(2, "-"),
		Type: "aws",
		Regions: []dbmodel.CloudRegion{{
			Name: petname.Generate(2, "-"),
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, gc.IsNil)
	id, _ := uuid.NewRandom()
	controller := dbmodel.Controller{
		Name:        petname.Generate(2, "-"),
		UUID:        id.String(),
		CloudName:   cloud.Name,
		CloudRegion: cloud.Regions[0].Name,
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	cred := dbmodel.CloudCredential{
		Name:              petname.Generate(2, "-"),
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, gc.IsNil)

	model := dbmodel.Model{
		Name: petname.Generate(2, "-"),
		UUID: sql.NullString{
			String: id.String(),
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
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

	err = ofgaClient.AddRelation(ctx,
		openfga.Tuple{
			Object:   ofganames.ConvertTag(u.ResourceTag()),
			Relation: "member",
			Target:   ofganames.ConvertTag(group.Tag().(jimmnames.GroupTag)),
		},
		openfga.Tuple{
			Object:   ofganames.ConvertTagWithRelation(group.Tag().(jimmnames.GroupTag), ofganames.MemberRelation),
			Relation: "reader",
			Target:   ofganames.ConvertTag(model.ResourceTag()),
		},
	)
	c.Assert(err, gc.IsNil)

	// Test reader is OK
	userToCheck := "user-" + u.Name
	modelToCheck := "model-" + u.Name + "/" + model.Name
	cmdCtx, err := cmdtesting.RunCommand(
		c,
		cmd.NewCheckRelationCommandForTesting(s.ClientStore(), bClient),
		userToCheck,
		"reader",
		modelToCheck,
	)

	c.Assert(err, gc.IsNil)

	c.Assert(
		strings.TrimRight(cmdtesting.Stdout(cmdCtx), "\n"),
		gc.Equals,
		fmt.Sprintf(cmd.AccessMessage, userToCheck, modelToCheck, "reader", cmd.AccessResultAllowed),
	)

	// Test writer is NOT OK
	cmdCtx, err = cmdtesting.RunCommand(
		c,
		cmd.NewCheckRelationCommandForTesting(s.ClientStore(), bClient),
		userToCheck,
		"writer",
		modelToCheck,
	)
	c.Assert(err, gc.IsNil)

	c.Assert(
		strings.TrimRight(cmdtesting.Stdout(cmdCtx), "\n"),
		gc.Equals,
		fmt.Sprintf(cmd.AccessMessage, userToCheck, modelToCheck, "writer", cmd.AccessResultDenied),
	)

	// Test format JSON
	cmdCtx, err = cmdtesting.RunCommand(
		c,
		cmd.NewCheckRelationCommandForTesting(s.ClientStore(), bClient),
		userToCheck,
		"reader",
		modelToCheck,
		"--format",
		"json",
	)
	c.Assert(err, gc.IsNil)

	res := cmdtesting.Stdout(cmdCtx)
	ar := cmd.AccessResult{}
	err = json.Unmarshal([]byte(res), &ar)
	c.Assert(err, gc.IsNil)
	b, err := json.Marshal(ar)
	c.Assert(err, gc.IsNil)

	c.Assert(
		strings.TrimRight(cmdtesting.Stdout(cmdCtx), "\n"),
		gc.Equals,
		string(b),
	)

	// Test format YAML
	cmdCtx, err = cmdtesting.RunCommand(
		c,
		cmd.NewCheckRelationCommandForTesting(s.ClientStore(), bClient),
		userToCheck,
		"reader",
		modelToCheck,
		"--format",
		"yaml",
	)
	c.Assert(err, gc.IsNil)

	// Create identical test output as we expect the CLI to return
	// via marshalling and umarshalling.
	res = cmdtesting.Stdout(cmdCtx)
	ar = cmd.AccessResult{}
	err = yamlv2.Unmarshal([]byte(res), &ar)
	c.Assert(err, gc.IsNil)
	b, err = yamlv2.Marshal(ar)
	c.Assert(err, gc.IsNil)

	c.Assert(
		cmdtesting.Stdout(cmdCtx),
		gc.Equals,
		string(b),
	)

}

func (s *relationSuite) TestCheckRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(
		c,
		cmd.NewCheckRelationCommandForTesting(s.ClientStore(), bClient),
		"user-diglett",
		"reader",
		"controller-jimm",
	)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
