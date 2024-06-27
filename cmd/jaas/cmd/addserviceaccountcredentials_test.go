// Copyright 2024 Canonical Ltd.

package cmd_test

import (
	"context"
	"fmt"

	"github.com/canonical/jimm/cmd/jaas/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"
)

type addServiceAccountCredentialSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&addServiceAccountCredentialSuite{})

func (s *addServiceAccountCredentialSuite) TestAddServiceAccountCredential(c *gc.C) {
	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIDWithDomain := clientID + "@serviceaccount"
	// Create Alice Identity and Service Account Identity.
	// alice is superuser
	ctx := context.Background()
	user, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)
	u := openfga.NewUser(user, s.OFGAClient)
	err = s.JIMM.AddServiceAccount(ctx, u, clientIDWithDomain)
	c.Assert(err, gc.IsNil)

	// Create cloud and cloud-credential for Alice.
	err = s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name:    "aws",
		Regions: []dbmodel.CloudRegion{{Name: "default", CloudName: "test-cloud"}},
	})
	c.Assert(err, gc.IsNil)
	updateArgs := jimm.UpdateCloudCredentialArgs{
		CredentialTag: names.NewCloudCredentialTag(fmt.Sprintf("aws/%s/foo", user.Name)),
		Credential:    params.CloudCredential{Attributes: map[string]string{"key": "bar"}},
	}
	_, err = s.JIMM.UpdateCloudCredential(ctx, u, updateArgs)
	c.Assert(err, gc.IsNil)
	bClient := jimmtest.NewUserSessionLogin(c, "alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewAddServiceAccountCredentialCommandForTesting(s.ClientStore(), bClient), clientID, "aws", "foo")
	c.Assert(err, gc.IsNil)
	newCred := dbmodel.CloudCredential{
		CloudName:         "aws",
		OwnerIdentityName: clientIDWithDomain,
		Name:              "foo",
	}
	err = s.JIMM.Database.GetCloudCredential(ctx, &newCred)
	c.Assert(err, gc.IsNil)
	// Verify the old credential's attribute map matches the new one.
	svcAcc, err := dbmodel.NewIdentity(clientIDWithDomain)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, svcAcc)
	c.Assert(err, gc.IsNil)
	svcAccIdentity := openfga.NewUser(svcAcc, s.OFGAClient)
	attr, _, err := s.JIMM.GetCloudCredentialAttributes(ctx, svcAccIdentity, &newCred, true)
	c.Assert(err, gc.IsNil)
	c.Assert(attr, gc.DeepEquals, updateArgs.Credential.Attributes)
}
