// Copyright 2023 Canonical Ltd.

package cmd_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
)

type relationSuite struct {
	jimmSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestAddRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
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
		{testName: "Add Group", input: tuple{user: "group-" + group1 + "#member", relation: "member", target: "group-" + group2}, err: false},
		{testName: "Invalid Relation", input: tuple{user: "group-" + group1 + "#member", relation: "admin", target: "group-" + group2}, err: true, message: "Invalid tuple"},
	}

	err := s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group1)
	c.Assert(err, gc.IsNil)
	err = s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group2)
	c.Assert(err, gc.IsNil)

	for i, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Log("Test: " + tc.testName)
		if tc.err {
			c.Assert(strings.Contains(err.Error(), tc.message), gc.Equals, true)
		} else {
			c.Assert(err, gc.IsNil)
			resp, err := s.jimmSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), nil, 50, "")
			c.Assert(err, gc.IsNil)
			c.Assert(len(resp.Keys), gc.Equals, i+1)
		}
	}

}

func (s *relationSuite) TestMissingParamsAddRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "foo", "bar")
	c.Assert(err, gc.ErrorMatches, "target object not specified")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "foo")
	c.Assert(err, gc.ErrorMatches, "relation not specified")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.ErrorMatches, "object not specified")

}

func (s *relationSuite) TestAddRelationViaFileSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
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
	testRelations := `[{"object":"group-` + group1 + `","relation":"member","target_object":"group-` + group3 + `"},{"object":"group-` + group2 + `","relation":"member","target_object":"group-` + group3 + `"}]`
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	resp, err := s.jimmSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), nil, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(resp.Keys), gc.Equals, 2)
}

func (s *relationSuite) TestAddRelationRejectsUnauthorisedUsers(c *gc.C) {
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *relationSuite) TestRemoveRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
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

	//Create groups and relation
	err := s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group1)
	c.Assert(err, gc.IsNil)
	err = s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group2)
	c.Assert(err, gc.IsNil)
	var totalKeys int
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
			resp, err := s.jimmSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), nil, 50, "")
			c.Assert(err, gc.IsNil)
			totalKeys--
			c.Assert(len(resp.Keys), gc.Equals, totalKeys)
		}
	}
}

func (s *relationSuite) TestRemoveRelationViaFileSuperuser(c *gc.C) {
	bClient := s.userBakeryClient("alice")
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
	testRelations := `[{"object":"group-` + group1 + `","relation":"member","target_object":"group-` + group3 + `"},{"object":"group-` + group2 + `","relation":"member","target_object":"group-` + group3 + `"}]`
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	resp, err := s.jimmSuite.JIMM.OpenFGAClient.ReadRelatedObjects(context.Background(), nil, 50, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(resp.Keys), gc.Equals, 0)
}

func (s *relationSuite) TestRemoveRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *relationSuite) TestCheckRelationViaSuperuser(c *gc.C) {
	ctx := context.TODO()
	bClient := s.userBakeryClient("alice")
	ofgaClient := s.JIMM.OpenFGAClient

	// Add some resources to check against
	db := s.JIMM.Database
	err := db.AddGroup(ctx, "test-group")
	c.Assert(err, gc.IsNil)
	group := dbmodel.GroupEntry{Name: "test-group"}
	err = db.GetGroup(ctx, &group)
	c.Assert(err, gc.IsNil)

	u := dbmodel.User{
		Username:         petname.Generate(2, "-") + "@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u).Error, gc.IsNil)

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
		Name:          petname.Generate(2, "-"),
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, gc.IsNil)

	model := dbmodel.Model{
		Name: petname.Generate(2, "-"),
		UUID: sql.NullString{
			String: id.String(),
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

	err = ofgaClient.AddRelations(ctx,
		ofga.CreateTupleKey("user:"+u.Username, "member", "group:"+strconv.FormatUint(uint64(group.ID), 10)),
		ofga.CreateTupleKey("group:"+strconv.FormatUint(uint64(group.ID), 10)+"#member", "reader", "model:"+model.UUID.String),
	)
	c.Assert(err, gc.IsNil)

	// Test reader is OK
	userToCheck := "user-" + u.Username
	modelToCheck := "model-" + controller.Name + ":" + u.Username + "/" + model.Name
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

}

func (s *relationSuite) TestCheckRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(
		c,
		cmd.NewCheckRelationCommandForTesting(s.ClientStore(), bClient),
		"diglett",
		"reader",
		"dugtrio",
	)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
