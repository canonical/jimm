// Copyright 2024 Canonical.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

type revokeAuditLogAccessSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&revokeAuditLogAccessSuite{})

func (s *revokeAuditLogAccessSuite) TestRevokeAuditLogAccessSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewRevokeAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@canonical.com")
	c.Assert(err, gc.IsNil)
}

func (s *revokeAuditLogAccessSuite) TestRevokeAuditLogAccess(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRevokeAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@canonical.com")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
