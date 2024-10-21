// Copyright 2024 Canonical.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type updateMigratedModelSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&updateMigratedModelSuite{})

func (s *updateMigratedModelSuite) TestUpdateMigratedModelSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	var model dbmodel.Model
	model.SetTag(mt)
	err := s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)
	s.AddController(c, "controller-2", s.APIInfo(c))

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewUpdateMigratedModelCommandForTesting(s.ClientStore(), bClient), "controller-2", mt.Id())
	c.Assert(err, gc.IsNil)

	// Check the model has moved controller.
	var model2 dbmodel.Model
	model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.ControllerID, gc.Not(gc.Equals), model.ControllerID)
}

func (s *updateMigratedModelSuite) TestUpdateMigratedModelUnauthorized(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateMigratedModelCommandForTesting(s.ClientStore(), bClient), "controller-1", mt.Id())
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *updateMigratedModelSuite) TestUpdateMigratedModelNoController(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateMigratedModelCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.ErrorMatches, `controller not specified`)
}

func (s *updateMigratedModelSuite) TestUpdateMigratedModelNoModelUUID(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateMigratedModelCommandForTesting(s.ClientStore(), bClient), "controller-id")
	c.Assert(err, gc.ErrorMatches, `model uuid not specified`)
}

func (s *updateMigratedModelSuite) TestUpdateMigratedModelInvalidModelUUID(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateMigratedModelCommandForTesting(s.ClientStore(), bClient), "controller-id", "not-a-uuid")
	c.Assert(err, gc.ErrorMatches, `invalid model uuid`)
}

func (s *updateMigratedModelSuite) TestUpdateMigratedModelTooManyArgs(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateMigratedModelCommandForTesting(s.ClientStore(), bClient), "controller-id", "not-a-uuid", "spare-argument")
	c.Assert(err, gc.ErrorMatches, `too many args`)
}
