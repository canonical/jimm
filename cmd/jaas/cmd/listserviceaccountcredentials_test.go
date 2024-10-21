// Copyright 2024 Canonical.

package cmd_test

import (
	"context"
	"fmt"

	jujucmd "github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

type listServiceAccountCredentialsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&listServiceAccountCredentialsSuite{})

func (s *listServiceAccountCredentialsSuite) TestListServiceAccountCredentials(c *gc.C) {
	// Add test cloud for cloud-credential to be valid.
	err := s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name:    "aws",
		Regions: []dbmodel.CloudRegion{{Name: "default", CloudName: "test-cloud"}},
	})
	c.Assert(err, gc.IsNil)
	// Create Alice Identity and Service Account Identity.
	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIDWithDomain := clientID + "@serviceaccount"
	// alice is superuser
	ctx := context.Background()
	user, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)
	u := openfga.NewUser(user, s.OFGAClient)
	err = s.JIMM.AddServiceAccount(ctx, u, clientIDWithDomain)
	c.Assert(err, gc.IsNil)
	svcAcc, err := dbmodel.NewIdentity(clientIDWithDomain)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, svcAcc)
	c.Assert(err, gc.IsNil)
	svcAccIdentity := openfga.NewUser(svcAcc, s.OFGAClient)
	// Create cloud-credential for service account.
	updateArgs := jimm.UpdateCloudCredentialArgs{
		CredentialTag: names.NewCloudCredentialTag(fmt.Sprintf("aws/%s/foo", clientIDWithDomain)),
		Credential:    params.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
	}
	_, err = s.JIMM.UpdateCloudCredential(ctx, svcAccIdentity, updateArgs)
	c.Assert(err, gc.IsNil)

	testCases := []struct {
		about       string
		showSecrets bool
		expected    string
		format      string
	}{
		{
			about:       "Tabular format output",
			showSecrets: false,
			expected: `
Controller Credentials:
Cloud  Credentials
aws    foo
`,
			format: "tabular",
		},
		{
			about:       "Yaml format output with secrets",
			showSecrets: true,
			expected: `controller-credentials:
  aws:
    foo:
      auth-type: ""
      foo: bar
`,
			format: "yaml",
		},
		{
			about:       "Yaml format output without secrets",
			showSecrets: false,
			expected: `controller-credentials:
  aws:
    foo:
      auth-type: ""
`,
			format: "yaml",
		},
		{
			about:       "JSON format output with secrets",
			showSecrets: true,
			expected:    `{\"controller-credentials\":{\"aws\":{\"cloud-credentials\":{\"foo\":{\"auth-type\":\"\",\"details\":{\"foo\":\"bar\"}}}}}}\n`,
			format:      "json",
		},
	}
	for _, test := range testCases {
		c.Log(test.about)
		bClient := s.SetupCLIAccess(c, "alice")
		var result *jujucmd.Context
		if test.showSecrets {
			result, err = cmdtesting.RunCommand(c, cmd.NewListServiceAccountCredentialsCommandForTesting(s.ClientStore(), bClient), clientID, "--format", test.format, "--show-secrets")
		} else {
			result, err = cmdtesting.RunCommand(c, cmd.NewListServiceAccountCredentialsCommandForTesting(s.ClientStore(), bClient), clientID, "--format", test.format)
		}
		c.Assert(err, gc.IsNil)
		c.Assert(cmdtesting.Stdout(result), gc.Matches, test.expected)
	}
}
