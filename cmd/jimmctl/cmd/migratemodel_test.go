// Copyright 2024 Canonical.

package cmd_test

import (
	"fmt"

	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type migrateModelSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&migrateModelSuite{})

// TestMigrateModelCommandSuperuser tests that a migration request makes it through to the Juju controller.
// Because our test suite only spins up 1 controller the furthest we can go is reaching Juju pre-checks which
// detect that a model with the same UUID already exists on the target controller.
// This functionality is already tested in jujuapi and ideally this test would only test the CLI functionality
// but our CLI tests are currently integration based.
func (s *migrateModelSuite) TestMigrateModelCommandSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))
	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-1", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	mt2 := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	context, err := cmdtesting.RunCommand(
		c, cmd.NewMigrateModelCommandForTesting(s.ClientStore(), bClient),
		"controller-1",
		mt.Id(),
		"charlie@canonical.com/model-2",
	)
	c.Assert(err, gc.IsNil)

	res := &jujuparams.InitiateMigrationResults{}
	out := cmdtesting.Stdout(context)
	err = yaml.Unmarshal([]byte(out), res)
	c.Assert(err, gc.IsNil)

	expected := "target prechecks failed: model with same UUID already exists (%s)"
	c.Assert(res.Results[0].Error.Message, gc.Equals, fmt.Sprintf(expected, mt.Id()))
	c.Assert(res.Results[1].Error.Message, gc.Equals, fmt.Sprintf(expected, mt2.Id()))
}

func (s *migrateModelSuite) TestMigrateModelCommandFailsWithMissingArgs(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewMigrateModelCommandForTesting(s.ClientStore(), bClient), "myController")
	c.Assert(err, gc.ErrorMatches, "Missing controller name and model target arguments")
}
