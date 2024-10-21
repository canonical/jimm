// Copyright 2024 Canonical.

package cmd_test

import (
	"context"
	"fmt"

	"github.com/juju/cmd/v3/cmdtesting"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type updateCredentialsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&updateCredentialsSuite{})

func (s *updateCredentialsSuite) TestUpdateCredentialsWithLocalCredentials(c *gc.C) {
	ctx := context.Background()

	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIDWithDomain := clientID + "@serviceaccount"

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	sa, err := dbmodel.NewIdentity(clientIDWithDomain)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, sa)
	c.Assert(err, gc.IsNil)

	// Make alice admin of the service account
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIDWithDomain)),
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

	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient), clientID, "test-cloud", "test-credentials", "--client")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, `results:
- credentialtag: cloudcred-test-cloud_abda51b2-d735-4794-a8bd-49c506baa4af@serviceaccount_test-credentials
  error: null
  models: []
`)

	ofgaUser := openfga.NewUser(sa, s.JIMM.AuthorizationClient())
	cloudCredentialTag := names.NewCloudCredentialTag("test-cloud/" + clientIDWithDomain + "/test-credentials")
	cloudCredential2, err := s.JIMM.GetCloudCredential(ctx, ofgaUser, cloudCredentialTag)
	c.Assert(err, gc.IsNil)
	attrs, _, err := s.JIMM.GetCloudCredentialAttributes(ctx, ofgaUser, cloudCredential2, true)
	c.Assert(err, gc.IsNil)

	c.Assert(attrs, gc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *updateCredentialsSuite) TestCloudNotInLocalStore(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient),
		"00000000-0000-0000-0000-000000000000",
		"non-existing-cloud",
		"foo",
		"--client",
	)
	c.Assert(err, gc.ErrorMatches, "failed to fetch local credentials for cloud \"non-existing-cloud\"")
}

func (s *updateCredentialsSuite) TestCredentialNotInLocalStore(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "alice")

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
		"--client",
	)
	c.Assert(err, gc.ErrorMatches, "credential \"non-existing-credential-name\" not found on local client.*")
}

func (s *updateCredentialsSuite) TestUpdateServiceAccountCredentialFromController(c *gc.C) {
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
	bClient := s.SetupCLIAccess(c, "alice")
	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient), clientID, "aws", "foo")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, `credentialtag: cloudcred-aws_abda51b2-d735-4794-a8bd-49c506baa4af@serviceaccount_foo
error: null
models: []
`)
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

func (s *updateCredentialsSuite) TestMissingArgs(c *gc.C) {
	tests := []struct {
		name          string
		args          []string
		expectedError string
	}{{
		name:          "missing client ID",
		args:          []string{},
		expectedError: "client ID not specified",
	}, {
		name:          "missing cloud",
		args:          []string{"some-client-id"},
		expectedError: "cloud not specified",
	}, {
		name:          "missing credential name",
		args:          []string{"some-client-id", "some-cloud"},
		expectedError: "credential name not specified",
	}, {
		name:          "too many args",
		args:          []string{"some-client-id", "some-cloud", "some-credential-name", "extra-arg"},
		expectedError: "too many args",
	}}

	bClient := s.SetupCLIAccess(c, "alice")
	clientStore := s.ClientStore()
	for _, t := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient), t.args...)
		c.Assert(err, gc.ErrorMatches, t.expectedError, gc.Commentf("test case failed: %q", t.name))
	}
}
