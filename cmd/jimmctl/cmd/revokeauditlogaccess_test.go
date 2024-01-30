// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
)

type revokeAuditLogAccessSuite struct {
	cmdtest.JimmCmdSuite
}

// TODO (alesstimec) uncomment when grant/revoke is implemented
//var _ = gc.Suite(&revokeAuditLogAccessSuite{})

func (s *revokeAuditLogAccessSuite) TestRevokeAuditLogAccessSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewRevokeAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@external")
	c.Assert(err, gc.IsNil)
}

func (s *revokeAuditLogAccessSuite) TestRevokeAuditLogAccess(c *gc.C) {
	// bob is not superuser
	bClient := s.UserBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRevokeAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@external")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
