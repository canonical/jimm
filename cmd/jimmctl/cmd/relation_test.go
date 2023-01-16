// Copyright 2023 Canonical Ltd.

package cmd_test

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

type relationSuite struct {
	jimmSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestAddRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
	type tuple struct {
		object   string
		relation string
		target   string
	}
	tests := []struct {
		testName string
		input    tuple
		err      bool
		message  string
	}{
		{testName: "Add Group", input: tuple{object: "group:group-test1#member", relation: "member", target: "group:group-test2"}, err: false},
		{testName: "Invalid Relation", input: tuple{object: "group:group-test1", relation: "admin", target: "group:group-test2"}, err: true, message: "invalid relation"},
	}

	err := s.jimmSuite.JIMM.Database.AddGroup(context.TODO(), "test1")
	c.Assert(err, gc.IsNil)
	err = s.jimmSuite.JIMM.Database.AddGroup(context.TODO(), "test2")
	c.Assert(err, gc.IsNil)

	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), tc.input.object, tc.input.relation, tc.input.target)
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

func (s *relationSuite) TestMissingParamsAddRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "foo", "bar")
	c.Assert(err, gc.ErrorMatches, "target object not specified")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "foo")
	c.Assert(err, gc.ErrorMatches, "relation not specified")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient))
	c.Assert(err, gc.ErrorMatches, "object not specified")

}

func (s *relationSuite) TestAddRelationViaFileSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")

	_, err := cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore, bClient), "test-group1")
	c.Assert(err, gc.IsNil)
	_, err = cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore, bClient), "test-group2")
	c.Assert(err, gc.IsNil)
	_, err = cmdtesting.RunCommand(c, cmd.NewAddGroupCommandForTesting(s.ClientStore, bClient), "test-group2")
	c.Assert(err, gc.IsNil)

	file, err := ioutil.TempFile(".", "relations.json")
	c.Assert(err, gc.IsNil)
	defer os.Remove(file.Name())
	testRelations := "[{\"object\":\"group:group-test1\",\"relation\":\"member\",\"target_object\":\"group:group-test3\"},{\"object\":\"group:group-test2\",\"relation\":\"member\",\"target_object\":\"group:group-test3\"}]"
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	//TODO:(Kian) a nice check here would be to use the CheckRelation method once it is implemented.
	//_, err = s.jimmSuite.JIMM.OpenFGAClient.CheckRelation(context.TODO(), "test-group", "test-group2")
	//c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *relationSuite) TestAddRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *relationSuite) TestRemoveRelationSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
	type tuple struct {
		object   string
		relation string
		target   string
	}
	tests := []struct {
		testName string
		input    tuple
		err      bool
		message  string
	}{
		{testName: "Add Group", input: tuple{object: "group:group-test1#member", relation: "member", target: "group:group-test2"}, err: false},
		{testName: "Invalid Relation", input: tuple{object: "group:group-test1", relation: "admin", target: "group:group-test2"}, err: true, message: "invalid relation"},
	}

	//Create groups and relation

	for _, tc := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore, bClient), tc.input.object, tc.input.relation, tc.input.target)
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

	//Add relations

	file, err := ioutil.TempFile(".", "relations.json")
	c.Assert(err, gc.IsNil)
	defer os.Remove(file.Name())
	testRelations := "[{\"object\":\"group:group-test1\",\"relation\":\"member\",\"target_object\":\"group:group-test3\"},{\"object\":\"group:group-test2\",\"relation\":\"member\",\"target_object\":\"group:group-test3\"}]"
	_, err = file.Write([]byte(testRelations))
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore, bClient), "-f", file.Name())
	c.Assert(err, gc.IsNil)

	//TODO:(Kian) a nice check here would be to use the CheckRelation method once it is implemented.
	//_, err = s.jimmSuite.JIMM.OpenFGAClient.CheckRelation(context.TODO(), "test-group", "test-group2")
	//c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *relationSuite) TestRemoveRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRemoveRelationCommandForTesting(s.ClientStore, bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
