// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/jimmtest"
)

type migrateModelSuite struct {
	jimmSuite
}

var _ = gc.Suite(&migrateModelSuite{})

var migrationResultRegex = `results:
- modeltag: ""
  error:
    message: 'target prechecks failed: model with same UUID already exists (.*)'
    code: ""
    info: {}
  migrationid: ""
- modeltag: ""
  error:
    message: 'target prechecks failed: model with same UUID already exists (.*)'
    code: ""
    info: {}
  migrationid: ""
`

// TestMigrateModelCommandSuperuser tests that a migration request makes it through to the Juju controller.
// Because our test suite only spins up 1 controller the furthest we can go is reaching Juju pre-checks which
// detect that a model with the same UUID already exists on the target controller.
// This functionality is already tested in jujuapi and ideally this test would only test the CLI functionality
// but our CLI tests are currently integration based.
func (s *migrateModelSuite) TestMigrateModelCommandSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))
	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-1", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	mt2 := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewMigrateModelCommandForTesting(s.ClientStore(), bClient), "controller-1", mt.String(), mt2.String())
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, migrationResultRegex)
}

func (s *migrateModelSuite) TestMigrateModelCommandFailsWithInvalidModelTag(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewMigrateModelCommandForTesting(s.ClientStore(), bClient), "controller-1", "model-001", "model-002")
	c.Assert(err, gc.ErrorMatches, ".* is not a valid model tag")
}

func (s *migrateModelSuite) TestMigrateModelCommandFailsWithMissingArgs(c *gc.C) {
	bClient := s.userBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewMigrateModelCommandForTesting(s.ClientStore(), bClient), "myController")
	c.Assert(err, gc.ErrorMatches, "Missing controller and model tag arguments")
}
