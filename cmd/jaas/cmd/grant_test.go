// Copyright 2024 Canonical.

package cmd_test

import (
	"context"
	"fmt"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type grantSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&grantSuite{})

func (s *grantSuite) TestGrant(c *gc.C) {
	ctx := context.Background()

	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIdWithDomain := clientID + "@serviceaccount"

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	sa, err := dbmodel.NewIdentity(clientIdWithDomain)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, sa)
	c.Assert(err, gc.IsNil)

	// Make alice admin of the service account
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIdWithDomain)),
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuple)
	c.Assert(err, gc.IsNil)

	_, err = s.JIMM.Database.AddGroup(ctx, "1")
	c.Assert(err, gc.IsNil)

	group := dbmodel.GroupEntry{
		Name: "1",
	}
	err = s.JIMM.Database.GetGroup(ctx, &group)
	c.Assert(err, gc.IsNil)

	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewGrantCommandForTesting(s.ClientStore(), bClient), clientID, "user-bob", "group-1")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, "access granted\n")

	ok, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIdWithDomain)),
	}, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)

	ok, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(jimmnames.NewGroupTag(fmt.Sprintf("%s#member", group.UUID))),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIdWithDomain)),
	}, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}

func (s *grantSuite) TestMissingArgs(c *gc.C) {
	tests := []struct {
		name          string
		args          []string
		expectedError string
	}{{
		name:          "missing client ID",
		args:          []string{},
		expectedError: "client ID not specified",
	}, {
		name:          "missing identity (user/group)",
		args:          []string{"some-client-id"},
		expectedError: "user/group not specified",
	}}

	bClient := s.SetupCLIAccess(c, "alice")
	clientStore := s.ClientStore()
	for _, t := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewGrantCommandForTesting(clientStore, bClient), t.args...)
		c.Assert(err, gc.ErrorMatches, t.expectedError, gc.Commentf("test case failed: %q", t.name))
	}
}
