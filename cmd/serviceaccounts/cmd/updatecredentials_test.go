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
	jujucloud "github.com/juju/juju/cloud"
)

type updateCredentialsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&updateCredentialsSuite{})

func (s *updateCredentialsSuite) TestUpdateCredentials(c *gc.C) {
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

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "kubernetes",
	}
	err = s.JIMM.Database.AddCloud(ctx, &cloud)
	c.Assert(err, gc.IsNil)

	cloudCredential := dbmodel.CloudCredential{
		Name:              "test-credentials",
		CloudName:         "test-cloud",
		OwnerIdentityName: clientID,
		AuthType:          "empty",
	}
	err = s.JIMM.Database.SetCloudCredential(ctx, &cloudCredential)
	c.Assert(err, gc.IsNil)

	clientStore := s.ClientStore()

	err = clientStore.UpdateCredential("test-cloud", jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"test-credentials": jujucloud.NewCredential(jujucloud.EmptyAuthType, map[string]string{
				"foo": "bar",
			}),
		},
	})
	c.Assert(err, gc.IsNil)

	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient), clientID, "test-cloud", "test-credentials")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, `results:
- credentialtag: cloudcred-test-cloud_abda51b2-d735-4794-a8bd-49c506baa4af_test-credentials
  error: null
  models: []
`)

	ofgaUser := openfga.NewUser(&sa, s.JIMM.AuthorizationClient())
	cloudCredentialTag := names.NewCloudCredentialTag("test-cloud/" + clientID + "/test-credentials")
	cloudCredential2, err := s.JIMM.GetCloudCredential(ctx, ofgaUser, cloudCredentialTag)
	c.Assert(err, gc.IsNil)
	attrs, _, err := s.JIMM.GetCloudCredentialAttributes(ctx, ofgaUser, cloudCredential2, true)
	c.Assert(err, gc.IsNil)

	c.Assert(attrs, gc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}
