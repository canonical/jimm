// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/jimmtest"
)

type revokeAuditLogAccessSuite struct {
	cmdtest.JimmCmdSuite
}

// TODO (alesstimec) uncomment when grant/revoke is implemented
//var _ = gc.Suite(&revokeAuditLogAccessSuite{})

func (s *revokeAuditLogAccessSuite) TestRevokeAuditLogAccessSuperuser(c *gc.C) {
	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewRevokeAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@canonical.com")
	c.Assert(err, gc.IsNil)
}

func (s *revokeAuditLogAccessSuite) TestRevokeAuditLogAccess(c *gc.C) {
	// bob is not superuser
	bClient := jimmtest.NewUserSessionLogin(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRevokeAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@canonical.com")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
