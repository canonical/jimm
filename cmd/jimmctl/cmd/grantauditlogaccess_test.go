// Copyright 2024 Canonical.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/cmdtest"
	"github.com/canonical/jimm/v3/internal/jimmtest"
)

type grantAuditLogAccessSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&grantAuditLogAccessSuite{})

func (s *grantAuditLogAccessSuite) TestGrantAuditLogAccessSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewGrantAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@canonical.com")
	c.Assert(err, gc.IsNil)
}

func (s *grantAuditLogAccessSuite) TestGrantAuditLogAccess(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewGrantAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@canonical.com")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
