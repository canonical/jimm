// Copyright 2025 Canonical.

package jujuapi_test

import (
	jimmversion "github.com/canonical/jimm/v3/version"
	"github.com/juju/juju/api/client/modelconfig"
	gc "gopkg.in/check.v1"
)

type modelConfigSuite struct {
	websocketSuite
}

var _ = gc.Suite(&modelConfigSuite{})

func (s *jimmSuite) TestModelGet(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := modelconfig.NewClient(conn)

	jimmCfg, err := client.ModelGet()
	c.Assert(err, gc.IsNil)

	v, ok := jimmCfg["agent-version"]
	c.Assert(ok, gc.Equals, true)
	vers, ok := v.(string)
	c.Assert(ok, gc.Equals, true)
	c.Assert(vers, gc.Equals, jimmversion.ControllerVersion)
}
