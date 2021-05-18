// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"github.com/juju/cmd/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

type disableControllerUUIDMaskingSuite struct {
	jimmSuite
}

var _ = gc.Suite(&disableControllerUUIDMaskingSuite{})

func (s *disableControllerUUIDMaskingSuite) TestDisableControllerUUIDMAskingSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewDisableControllerUUIDMaskingCommandForTesting(s.ClientStore, bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, ``)
}

func (s *disableControllerUUIDMaskingSuite) TestDisableControllerUUIDMAsking(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	context, err := cmdtesting.RunCommand(c, cmd.NewDisableControllerUUIDMaskingCommandForTesting(s.ClientStore, bClient))
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, ``)
}
