// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/jimmtest"
)

type listAuditEventsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&listAuditEventsSuite{})

func (s *listAuditEventsSuite) TestListAuditEventsSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewListAuditEventsCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches,
		`events:
- time: .*
  conversation-id: .*
  message-id: 1
  facade-name: Admin
  facade-method: Login
  facade-version: \d
  user-tag: user-
  is-response: false
  params:
    params: redacted
- time: .*
  conversation-id: .*
  message-id: 1
  facade-name: Admin
  facade-method: Login
  facade-version: \d
  user-tag: user-
  is-response: true
  errors:
    results:
    - error:
        code: ""
        message: ""
[\s\S]*`)
}

func (s *listAuditEventsSuite) TestListAuditEventsStatus(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewListAuditEventsCommandForTesting(s.ClientStore(), bClient))
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
