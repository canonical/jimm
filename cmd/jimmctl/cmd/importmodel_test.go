// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
)

type importModelSuite struct {
	jimmSuite
}

var _ = gc.Suite(&importModelSuite{})

func (s *importModelSuite) TestImportModelSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	var model dbmodel.Model
	model.SetTag(mt)
	err := s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient), "controller-1", mt.Id())
	c.Assert(err, gc.IsNil)

	var model2 dbmodel.Model
	model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.CreatedAt.After(model.CreatedAt), gc.Equals, true)
}

func (s *importModelSuite) TestImportModelFromLocalUser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/local-user/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AdminUser = &dbmodel.User{
		Username:         "local-user",
		ControllerAccess: "superuser",
		LastLogin:        db.Now(),
	}
	err := s.JIMM.Database.GetUser(context.Background(), s.AdminUser)
	c.Assert(err, gc.Equals, nil)
	mt := s.AddModel(c, names.NewUserTag("local-user"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	var model dbmodel.Model
	model.SetTag(mt)
	err = s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient), "controller-1", mt.Id(), "--owner", "alice@external")
	c.Assert(err, gc.IsNil)

	var model2 dbmodel.Model
	model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.CreatedAt.After(model.CreatedAt), gc.Equals, true)
}

func (s *importModelSuite) TestImportModelUnauthorized(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	var model dbmodel.Model
	model.SetTag(mt)
	err := s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), &model)
	c.Assert(err, gc.Equals, nil)

	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err = cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient), "controller-1", mt.Id())
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *importModelSuite) TestImportModelNoController(c *gc.C) {
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient))
	c.Assert(err, gc.ErrorMatches, `controller not specified`)
}

func (s *importModelSuite) TestImportModelNoModelUUID(c *gc.C) {
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient), "controller-id")
	c.Assert(err, gc.ErrorMatches, `model uuid not specified`)
}

func (s *importModelSuite) TestImportModelInvalidModelUUID(c *gc.C) {
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient), "controller-id", "not-a-uuid")
	c.Assert(err, gc.ErrorMatches, `invalid model uuid`)
}

func (s *importModelSuite) TestImportModelTooManyArgs(c *gc.C) {
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewImportModelCommandForTesting(s.ClientStore, bClient), "controller-id", "not-a-uuid", "spare-argument")
	c.Assert(err, gc.ErrorMatches, `too many args`)
}
