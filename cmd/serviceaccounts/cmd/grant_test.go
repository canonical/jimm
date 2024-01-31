// Copyright 2024 Canonical Ltd.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/serviceaccounts/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

type grantSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&grantSuite{})

func (s *grantSuite) TestGrant(c *gc.C) {
	ctx := context.Background()

	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"

	// alice is superuser
	bClient := s.UserBakeryClient("alice")

	sa := dbmodel.Identity{
		Name: clientID,
	}
	err := s.JIMM.Database.GetIdentity(ctx, &sa)
	c.Assert(err, gc.IsNil)

	// Make alice admin of the service account
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@external")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientID)),
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuple)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.AddGroup(ctx, "1")
	c.Assert(err, gc.IsNil)

	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewGrantCommandForTesting(s.ClientStore(), bClient), clientID, "user-bob", "group-1")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, "access granted\n")

	ok, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientID)),
	}, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)

	ok, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(jimmnames.NewGroupTag("1#member")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientID)),
	}, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}
