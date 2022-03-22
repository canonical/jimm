// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
)

type controllerrootSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerrootSuite{})

func (s *controllerrootSuite) TestServerVersion(c *gc.C) {
	ctx := context.Background()

	s.Model.Controller.AgentVersion = "5.4.3"
	err := s.JIMM.Database.UpdateController(ctx, &s.Model.Controller)
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	v, ok := conn.ServerVersion()
	c.Assert(ok, gc.Equals, true)
	c.Assert(v, jc.DeepEquals, version.MustParse("5.4.3"))
}

func (s *controllerrootSuite) TestUnimplementedMethodFails(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.Tag().(names.ModelTag),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "Logout", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin\(3\).Logout is not implemented \(not implemented\)`)
}

func (s *controllerrootSuite) TestUnimplementedRootFails(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method NoSuch\(1\).Method is not implemented \(not implemented\)`)
}
