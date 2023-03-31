// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"
)

type adminSuite struct {
	websocketSuite
}

var _ = gc.Suite(&adminSuite{})

func (s *adminSuite) TestLoginToController(c *gc.C) {
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.Equals, nil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotImplemented)
}

func (s *adminSuite) TestLoginToControllerWithInvalidMacaroon(c *gc.C) {
	invalidMacaroon, err := macaroon.New(nil, []byte("invalid"), "", macaroon.V1)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		Macaroons: []macaroon.Slice{{invalidMacaroon}},
	}, "test")
	conn.Close()
}
