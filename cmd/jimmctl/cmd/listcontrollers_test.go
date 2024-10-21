// Copyright 2024 Canonical.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

var (
	expectedSuperuserOutput = `- name: controller-1
  uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  publicaddress: ""
  apiaddresses:
  - localhost:.*
  cacertificate: |
    -----BEGIN CERTIFICATE-----
    .*
    -----END CERTIFICATE-----
  cloudtag: cloud-` + jimmtest.TestCloudName + `
  cloudregion: ` + jimmtest.TestCloudRegionName + `
  username: admin
  agentversion: .*
  status:
    status: available
    info: ""
    data: {}
    since: null
- name: controller-1
  uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  publicaddress: ""
  apiaddresses:
  - localhost:46539
  cacertificate: |
    -----BEGIN CERTIFICATE-----
    .*
    -----END CERTIFICATE-----
  cloudtag: cloud-` + jimmtest.TestCloudName + `
  cloudregion: ` + jimmtest.TestCloudRegionName + `
  username: admin
  agentversion: .*
  status:
    status: available
    info: ""
    data: {}
    since: null
`

	expectedOutput = `- name: jaas
  uuid: 6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11
  publicaddress: ""
  apiaddresses: \[\]
  cacertificate: ""
  cloudtag: ""
  cloudregion: ""
  username: ""
  agentversion: .*
  status:
    status: available
    info: ""
    data: {}
    since: null
`
)

type listControllersSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&listControllersSuite{})

func (s *listControllersSuite) TestListControllersSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewListControllersCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, expectedSuperuserOutput)
}

func (s *listControllersSuite) TestListControllers(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	context, err := cmdtesting.RunCommand(c, cmd.NewListControllersCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, expectedOutput)
}
