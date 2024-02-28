// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/jimmtest"
)

type grantAuditLogAccessSuite struct {
	cmdtest.JimmCmdSuite
}

// TODO (alesstimec) uncomment once granting/revoking is reimplemented
//var _ = gc.Suite(&grantAuditLogAccessSuite{})

func (s *grantAuditLogAccessSuite) TestGrantAuditLogAccessSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewGrantAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@external")
	c.Assert(err, gc.IsNil)
}

func (s *grantAuditLogAccessSuite) TestGrantAuditLogAccess(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewGrantAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@external")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
