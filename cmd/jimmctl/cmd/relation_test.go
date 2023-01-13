// Copyright 2023 Canonical Ltd.

package cmd_test

import (
	"context"

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

	//err := s.jimmSuite.JIMM.Database.AddRelation(context.TODO(), "test-group")
	//c.Assert(err, gc.IsNil)

	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.IsNil)

	_, err = s.jimmSuite.JIMM.Database.GetGroup(context.TODO(), "test-group")
	c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *relationSuite) TestAddRelationViaFileSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")

	//err := s.jimmSuite.JIMM.Database.AddRelation(context.TODO(), "test-group")
	//c.Assert(err, gc.IsNil)

	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "-f", "./local/openfga/tuples.json")
	c.Assert(err, gc.IsNil)

	_, err = s.jimmSuite.JIMM.Database.GetGroup(context.TODO(), "test-group")
	c.Assert(err, gc.ErrorMatches, "record not found")
}

func (s *relationSuite) TestAddRelation(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddRelationCommandForTesting(s.ClientStore, bClient), "test-group1", "member", "test-group2")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
