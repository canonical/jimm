// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/serviceaccounts/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

type addServiceAccountSuite struct {
	cmdtest.JimmSuite
}

var _ = gc.Suite(&addServiceAccountSuite{})

func (s *addServiceAccountSuite) TestAddServiceAccount(c *gc.C) {
	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	// alice is superuser
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCommandForTesting(s.ClientStore(), bClient), clientID)
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("user-alice@external")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientID)),
	}
	ok, err := s.JIMM.OpenFGAClient.CheckRelation(context.Background(), tuple, false)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, true)
}
