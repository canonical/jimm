// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"
	"gopkg.in/macaroon.v2"
)

type adminSuite struct {
	websocketSuite
}

var _ = gc.Suite(&adminSuite{})

func (s *adminSuite) TestOldAdminVersionFails(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(s.Model.UUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JIMM does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *adminSuite) TestAdminIDFails(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(s.Model.UUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "Object ID", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "id not found")
}

func (s *adminSuite) TestLoginToController(c *gc.C) {
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.Equals, nil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	rerr, ok := errgo.Cause(err).(*rpc.RequestError)
	c.Assert(ok, gc.Equals, true)
	c.Assert(rerr.Code, gc.Equals, jujuparams.CodeNotImplemented)
}

func (s *adminSuite) TestLoginToControllerWithInvalidMacaroon(c *gc.C) {
	invalidMacaroon, err := macaroon.New(nil, []byte("invalid"), "", macaroon.V1)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		Macaroons: []macaroon.Slice{{invalidMacaroon}},
	}, "test")
	conn.Close()
}

type modelAdminSuite struct {
	websocketSuite
}

var _ = gc.Suite(&modelAdminSuite{})

func (s *modelAdminSuite) TestLoginToModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(s.Model.UUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	nphps, err := network.ParseProviderHostPorts(s.APIInfo(c).Addrs...)
	c.Assert(err, gc.Equals, nil)
	nmhps := make(network.MachineHostPorts, len(nphps))
	// Change all unknown scopes to public.
	for i := range nphps {
		nmhps[i] = network.MachineHostPort{
			MachineAddress: nphps[i].MachineAddress,
			NetPort:        nphps[i].NetPort,
		}
		if nmhps[i].Scope == network.ScopeUnknown {
			nmhps[i].Scope = network.ScopePublic
		}
	}
	err = conn.Login(nil, "", "", nil)
	c.Assert(errgo.Cause(err), jc.DeepEquals, &api.RedirectError{
		Servers:        []network.MachineHostPorts{nmhps},
		CACert:         s.APIInfo(c).CACert,
		FollowRedirect: true,
	})
}

func (s *modelAdminSuite) TestOldAdminVersionFails(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(s.Model.UUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JIMM does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *modelAdminSuite) TestAdminIDFails(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(s.Model.UUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "Object ID", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "id not found")
}
