// Copyright 2024 Canonical.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

type importCloudCredentialsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&importCloudCredentialsSuite{})

//nolint:gosec // Thinks hardcoded creds.
const creds = `{
	"_id": "aws/alice@canonical.com/test1",
	"type": "access-key",
	"attributes": {
		"access-key": "key-id",
		"secret-key": "shhhh"
	}
}
{
	"_id": "aws/bob@canonical.com/test1",
	"type": "access-key",
	"attributes": {
		"access-key": "key-id2",
		"secret-key": "shhhh"
	}
}
{
	"_id": "gce/charlie@canonical.com/test1",
	"type": "empty",
	"attributes": {}
}`

func (s *importCloudCredentialsSuite) TestImportCloudCredentials(c *gc.C) {
	err := s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name:    "aws",
		Type:    "kubernetes",
		Regions: []dbmodel.CloudRegion{{Name: "default", CloudName: "test-cloud"}},
	})
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name:    "gce",
		Type:    "kubernetes",
		Regions: []dbmodel.CloudRegion{{Name: "default", CloudName: "test-cloud"}},
	})
	c.Assert(err, gc.IsNil)

	tmpfile := filepath.Join(c.MkDir(), "test.json")
	err = os.WriteFile(tmpfile, []byte(creds), 0600)
	c.Assert(err, gc.IsNil)

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportCloudCredentialsCommandForTesting(s.ClientStore(), bClient), tmpfile)
	c.Assert(err, gc.IsNil)

	cred1 := dbmodel.CloudCredential{
		CloudName:         "aws",
		OwnerIdentityName: "alice@canonical.com",
		Name:              "test1",
	}
	err = s.JIMM.Database.GetCloudCredential(context.Background(), &cred1)
	c.Assert(err, gc.IsNil)
	c.Check(cred1.AuthType, gc.Equals, "access-key")

	cred2 := dbmodel.CloudCredential{
		CloudName:         "aws",
		OwnerIdentityName: "bob@canonical.com",
		Name:              "test1",
	}
	err = s.JIMM.Database.GetCloudCredential(context.Background(), &cred2)
	c.Assert(err, gc.IsNil)
	c.Check(cred2.AuthType, gc.Equals, "access-key")

	cred3 := dbmodel.CloudCredential{
		CloudName:         "gce",
		OwnerIdentityName: "charlie@canonical.com",
		Name:              "test1",
	}
	err = s.JIMM.Database.GetCloudCredential(context.Background(), &cred3)
	c.Assert(err, gc.IsNil)
	c.Check(cred3.AuthType, gc.Equals, "empty")
}
