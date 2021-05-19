// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/cmdtesting"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

type listAuditEventsSuite struct {
	jimmSuite
}

var _ = gc.Suite(&listAuditEventsSuite{})

func (s *listAuditEventsSuite) TestListAuditEventsSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag("dummy/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag("dummy"), "dummy-region", cct)

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewListAuditEventsCommandForTesting(s.ClientStore, bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, `events:
- time: .*
  tag: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
  usertag: user-alice@external
  action: add
  success: true
  params:
    name: controller-1
- time: .*
  tag: cloudcred-dummy_charlie@external_cred
  usertag: user-charlie@external
  action: update
  success: true
  params:
    skip-check: \"true\"
    skip-update: \"false\"
- time: .*
  tag: model-.*
  usertag: user-charlie@external
  action: create
  success: true
  params:
    cloud: cloud-dummy
    cloud-credential: cloudcred-dummy_charlie@external_cred
    name: model-2
    owner: user-charlie@external
    region: dummy-region
`)
}

func (s *listAuditEventsSuite) TestListAuditEventsStatus(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag("dummy/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag("dummy"), "dummy-region", cct)

	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewListAuditEventsCommandForTesting(s.ClientStore, bClient))
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
