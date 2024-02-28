// Copyright 2024 Canonical Ltd.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jaas/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

type addServiceAccountSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&addServiceAccountSuite{})

func (s *addServiceAccountSuite) TestAddServiceAccount(c *gc.C) {
	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	// alice is superuser
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClient), clientID)
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@external")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientID)),
	}
	// Check alice has access.
	ok, err := s.JIMM.OpenFGAClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
	// Check that re-running the command doesn't return an error for Alice.
	_, err = cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClient), clientID)
	c.Assert(err, gc.IsNil)
	// Check that re-running the command for a different user returns an error.
	bClientBob := s.UserBakeryClient("bob")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClientBob), clientID)
	c.Assert(err, gc.ErrorMatches, "service account already owned")
}
