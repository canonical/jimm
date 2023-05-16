// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

type importCloudCredentialsSuite struct {
	jimmSuite
}

var _ = gc.Suite(&importCloudCredentialsSuite{})

const creds = `{
	"_id": "aws/alice/test1",
	"type": "access-key",
	"attributes": {
		"access-key": "key-id",
		"secret-key": "shhhh"
	}
}
{
	"_id": "aws/bob@external/test1",
	"type": "access-key",
	"attributes": {
		"access-key": "key-id2",
		"secret-key": "shhhh"
	}
}
{
	"_id": "gce/charlie/test1",
	"type": "empty",
	"attributes": {}
}`

func (s *importCloudCredentialsSuite) TestImportCloudCredentials(c *gc.C) {
	tmpfile := filepath.Join(c.MkDir(), "test.json")
	err := os.WriteFile(tmpfile, []byte(creds), 0660)
	c.Assert(err, gc.IsNil)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportCloudCredentialsCommandForTesting(s.ClientStore(), bClient), tmpfile)
	c.Assert(err, gc.IsNil)

	cred1 := dbmodel.CloudCredential{
		CloudName:     "aws",
		OwnerUsername: "alice@external",
		Name:          "test1",
	}
	err = s.JIMM.Database.GetCloudCredential(context.Background(), &cred1)
	c.Assert(err, gc.IsNil)
	c.Check(cred1.AuthType, gc.Equals, "access-key")

	cred2 := dbmodel.CloudCredential{
		CloudName:     "aws",
		OwnerUsername: "bob@external",
		Name:          "test1",
	}
	err = s.JIMM.Database.GetCloudCredential(context.Background(), &cred2)
	c.Assert(err, gc.IsNil)
	c.Check(cred2.AuthType, gc.Equals, "access-key")

	cred3 := dbmodel.CloudCredential{
		CloudName:     "gce",
		OwnerUsername: "charlie@external",
		Name:          "test1",
	}
	err = s.JIMM.Database.GetCloudCredential(context.Background(), &cred3)
	c.Assert(err, gc.IsNil)
	c.Check(cred3.AuthType, gc.Equals, "empty")
}