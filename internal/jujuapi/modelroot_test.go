// Copyright 2017 Canonical Ltd.

package jujuapi_test

import (
	"github.com/juju/juju/api"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"
)

type modelSuite struct {
	websocketSuite
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) TestUnknownModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag("00000000-0000-0000-0000-000000000000"),
		SkipLogin: true,
	}, "bob")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.ErrorMatches, `model not found \(model not found\)`)
}
