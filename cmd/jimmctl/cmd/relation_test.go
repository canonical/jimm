// Copyright 2023 Canonical Ltd.

package cmd_test

import (
	"context"
	"os"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

type relationSuite struct {
	fgaSuite
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
		{testName: "Add Group", input: tuple{user: "group:group-" + group1 + "#member", relation: "member", target: "group:group-" + group2}, err: false},
		{testName: "Invalid Relation", input: tuple{user: "group:group-" + group1 + "#member", relation: "admin", target: "group:group-" + group2}, err: true, message: "Invalid tuple"},
	}

	err := s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group1)
	c.Assert(err, gc.IsNil)
	err = s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group2)
	c.Assert(err, gc.IsNil)

	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Log("Test: " + tc.testName)
		if tc.err {
			c.Assert(strings.Contains(err.Error(), tc.message), gc.Equals, true)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}

	//TODO:(Kian) a nice check here would be to use the CheckRelation method once it is implemented.
	//_, err = s.jimmSuite.JIMM.OpenFGAClient.CheckRelation(context.TODO(), "test-group", "test-group2")
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
	testRelations := "[{\"object\":\"group:group-" + group1 + "\",\"relation\":\"member\",\"target_object\":\"group:group-" + group3 + "\"},{\"object\":\"group:group-" + group2 + "\",\"relation\":\"member\",\"target_object\":\"group:group-" + group3 + "\"}]"
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	//TODO:(Kian) a nice check here would be to use the CheckRelation method once it is implemented.
	//_, err = s.jimmSuite.JIMM.OpenFGAClient.CheckRelation(context.TODO(), "test-group", "test-group2")
	//c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *relationSuite) TestAddRelation(c *gc.C) {
	// bob is not superuser
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
		{testName: "Remove Group Relation", input: tuple{user: "group:group-" + group1 + "#member", relation: "member", target: "group:group-" + group2}, err: false},
	}

	//Create groups and relation
	err := s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group1)
	c.Assert(err, gc.IsNil)
	err = s.jimmSuite.JIMM.Database.AddGroup(context.Background(), group2)
	c.Assert(err, gc.IsNil)
	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Assert(err, gc.IsNil)
	}

	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), tc.input.user, tc.input.relation, tc.input.target)
		c.Log("Test: " + tc.testName)
		if tc.err {
			c.Assert(err, gc.ErrorMatches, tc.message)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}

	//TODO:(Kian) a nice check here would be to use the CheckRelation method once it is implemented.
	//_, err = s.jimmSuite.JIMM.OpenFGAClient.CheckRelation(context.TODO(), "test-group", "test-group2")
}

func (s *relationSuite) TestRemoveRelationViaFileSuperuser(c *gc.C) {
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
	testRelations := "[{\"object\":\"group:group-" + group1 + "\",\"relation\":\"member\",\"target_object\":\"group:group-" + group3 + "\"},{\"object\":\"group:group-" + group2 + "\",\"relation\":\"member\",\"target_object\":\"group:group-" + group3 + "\"}]"
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	//TODO:(Kian) a nice check here would be to use the CheckRelation method once it is implemented.
	//_, err = s.jimmSuite.JIMM.OpenFGAClient.CheckRelation(context.TODO(), "test-group", "test-group2")
	//c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *relationSuite) TestRemoveRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore(), bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
