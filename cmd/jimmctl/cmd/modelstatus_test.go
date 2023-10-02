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

var (
	expectedModelStatusOutput = `model:
  name: model-2
  type: iaas
  cloudtag: cloud-` + jimmtest.TestCloudName + `
  cloudregion: ` + jimmtest.TestCloudRegionName + `
  version: .*
  availableversion: ""
  modelstatus:
    status: available
    info: ""
    data: {}
    since: .*
    kind: ""
    version: ""
    life: ""
    err: null
  meterstatus:
    color: ""
    message: ""
  sla: unsupported
machines: {}
applications: {}
remoteapplications: {}
offers: {}
relations: \[\]
controllertimestamp: .*
branches: {}
`
)

type modelStatusSuite struct {
	jimmSuite
}

var _ = gc.Suite(&modelStatusSuite{})

func (s *modelStatusSuite) TestModelStatusSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty", Attributes: map[string]string{"key": "value"}})
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewModelStatusCommandForTesting(s.ClientStore(), bClient), mt.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, expectedModelStatusOutput)
}

func (s *modelStatusSuite) TestModelStatus(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty", Attributes: map[string]string{"key": "value"}})
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewModelStatusCommandForTesting(s.ClientStore(), bClient), mt.Id())
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
