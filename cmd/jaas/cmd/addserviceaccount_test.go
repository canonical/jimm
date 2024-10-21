// Copyright 2024 Canonical.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type addServiceAccountSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&addServiceAccountSuite{})

func (s *addServiceAccountSuite) TestAddServiceAccount(c *gc.C) {
	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIDWithDomain := clientID + "@serviceaccount"
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClient), clientID)
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIDWithDomain)),
	}
	// Check alice has access.
	ok, err := s.JIMM.OpenFGAClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
	// Check that re-running the command doesn't return an error for Alice.
	_, err = cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClient), clientID)
	c.Assert(err, gc.IsNil)
	// Check that re-running the command for a different user returns an error.
	bClientBob := s.SetupCLIAccess(c, "bob")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClientBob), clientID)
	c.Assert(err, gc.ErrorMatches, "service account already owned")
}
