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

func (s *updateCredentialsSuite) TestUpdateCredentialsWithNewCredentials(c *gc.C) {
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

func (s *updateCredentialsSuite) TestUpdateCredentialsWithExistingCredentials(c *gc.C) {
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

func (s *updateCredentialsSuite) TestCloudNotInLocalStore(c *gc.C) {
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient),
		"00000000-0000-0000-0000-000000000000",
		"non-existing-cloud",
		"foo",
	)
	c.Assert(err, gc.ErrorMatches, "failed to fetch local credentials for cloud \"non-existing-cloud\"")
}

func (s *updateCredentialsSuite) TestCredentialNotInLocalStore(c *gc.C) {
	bClient := s.UserBakeryClient("alice")

	clientStore := s.ClientStore()
	err := clientStore.UpdateCredential("some-cloud", jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"some-credentials": jujucloud.NewCredential(jujucloud.EmptyAuthType, nil),
		},
	})
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient),
		"00000000-0000-0000-0000-000000000000",
		"some-cloud",
		"non-existing-credential-name",
	)
	c.Assert(err, gc.ErrorMatches, "credential \"non-existing-credential-name\" not found on local client.*")
}

func (s *updateCredentialsSuite) TestMissingClientIDArg(c *gc.C) {
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.ErrorMatches, "client ID not specified")
}

func (s *updateCredentialsSuite) TestMissingCloudArg(c *gc.C) {
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient),
		"some-client-id",
	)
	c.Assert(err, gc.ErrorMatches, "cloud not specified")
}

func (s *updateCredentialsSuite) TestMissingCredentialNameArg(c *gc.C) {
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient),
		"some-client-id",
		"some-cloud",
	)
	c.Assert(err, gc.ErrorMatches, "credential name not specified")
}

func (s *updateCredentialsSuite) TestTooManyArgs(c *gc.C) {
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient),
		"some-client-id",
		"some-cloud",
		"some-credential-name",
		"extra-arg",
	)
	c.Assert(err, gc.ErrorMatches, "too many args")
}
