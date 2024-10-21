// Copyright 2024 Canonical.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	jjcloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type importModelSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&importModelSuite{})

func (s *importModelSuite) TestImportModelSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty", Attributes: map[string]string{"key": "value"}})

	err := s.BackingState.UpdateCloudCredential(cct, jjcloud.NewCredential(jjcloud.EmptyAuthType, map[string]string{"key": "value"}))
	c.Assert(err, gc.Equals, nil)

	m := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "model-2",
		Owner:           names.NewUserTag("charlie@canonical.com"),
		CloudName:       jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: cct,
	})
	defer m.Close()

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient), "controller-1", m.ModelUUID())
	c.Assert(err, gc.IsNil)

	var model2 dbmodel.Model
	model2.SetTag(names.NewModelTag(m.ModelUUID()))
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.OwnerIdentityName, gc.Equals, "charlie@canonical.com")
}

func (s *importModelSuite) TestImportModelFromLocalUser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))
	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	// Add credentials for Alice on the test cloud, they are needed for the Alice user to become the new model owner
	cctAlice := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/alice@canonical.com/cred")
	s.UpdateCloudCredential(c, cctAlice, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	var model dbmodel.Model
	model.SetTag(mt)
	err := s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.OpenFGAClient.RemoveControllerModel(context.Background(), model.Controller.ResourceTag(), model.ResourceTag())
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient), "controller-1", mt.Id(), "--owner", "alice@canonical.com")
	c.Assert(err, gc.IsNil)

	var model2 dbmodel.Model
	model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.CreatedAt.After(model.CreatedAt), gc.Equals, true)
	c.Check(model2.OwnerIdentityName, gc.Equals, "alice@canonical.com")
}

func (s *importModelSuite) TestImportModelUnauthorized(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})

	err := s.BackingState.UpdateCloudCredential(cct, jjcloud.NewCredential(jjcloud.EmptyAuthType, map[string]string{"key": "value"}))
	c.Assert(err, gc.Equals, nil)

	m := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "model-2",
		Owner:           names.NewUserTag("charlie@canonical.com"),
		CloudName:       jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: cct,
	})
	defer m.Close()

	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient), "controller-1", m.ModelUUID())
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *importModelSuite) TestImportModelNoController(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.ErrorMatches, `controller not specified`)
}

func (s *importModelSuite) TestImportModelNoModelUUID(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient), "controller-id")
	c.Assert(err, gc.ErrorMatches, `model uuid not specified`)
}

func (s *importModelSuite) TestImportModelInvalidModelUUID(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient), "controller-id", "not-a-uuid")
	c.Assert(err, gc.ErrorMatches, `invalid model uuid`)
}

func (s *importModelSuite) TestImportModelTooManyArgs(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore(), bClient), "controller-id", "not-a-uuid", "spare-argument")
	c.Assert(err, gc.ErrorMatches, `too many args`)
}
