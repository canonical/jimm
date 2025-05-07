// Copyright 2025 Canonical.

package jujuclient_test

import (
	"context"

	gc "gopkg.in/check.v1"
)

type controllerSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&controllerSuite{})

func (cs *controllerSuite) TestControllerConfig(c *gc.C) {
	// Test the ControllerConfig method
	controllerConfig, err := cs.API.ControllerConfig(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(controllerConfig, gc.NotNil)
	c.Assert(controllerConfig.Config, gc.NotNil)
	c.Assert(controllerConfig.Config["controller-uuid"], gc.Equals, cs.ControllerConfig.ControllerUUID())
}
