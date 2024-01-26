// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
)

type grantAuditLogAccessSuite struct {
	cmdtest.JimmSuite
}

// TODO (alesstimec) uncomment once granting/revoking is reimplemented
//var _ = gc.Suite(&grantAuditLogAccessSuite{})

func (s *grantAuditLogAccessSuite) TestGrantAuditLogAccessSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.UserBakeryClient("alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewGrantAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@external")
	c.Assert(err, gc.IsNil)
}

func (s *grantAuditLogAccessSuite) TestGrantAuditLogAccess(c *gc.C) {
	// bob is not superuser
	bClient := s.UserBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewGrantAuditLogAccessCommandForTesting(s.ClientStore(), bClient), "bob@external")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
