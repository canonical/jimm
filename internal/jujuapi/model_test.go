// Copyright 2017 Canonical Ltd.

package jujuapi_test

import (
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jem/params"
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
	c.Assert(err, gc.ErrorMatches, `model "00000000-0000-0000-0000-000000000000" not found \(model not found\)`)
}

func (s *modelSuite) TestLoginToModel(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	nhps, err := network.ParseHostPorts(s.APIInfo(c).Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Change all unknown scopes to public.
	for i := range nhps {
		nhp := &nhps[i]
		if nhp.Scope == network.ScopeUnknown {
			nhp.Scope = network.ScopePublic
		}
	}
	err = conn.Login(nil, "", "", nil)
	c.Assert(errgo.Cause(err), jc.DeepEquals, &api.RedirectError{
		Servers: [][]network.HostPort{nhps},
		CACert:  s.APIInfo(c).CACert,
	})
}

func (s *modelSuite) TestOldAdminVersionFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JIMM does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *modelSuite) TestAdminIDFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "Object ID", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "id not found")
}

func (s *modelSuite) TestUnimplementedMethodFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.APICall("Admin", 3, "", "Logout", nil, nil)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.Logout is not implemented \(not implemented\)`)
}

func (s *modelSuite) TestUnimplementedRootFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.APICall("NoSuch", 1, "", "Method", nil, nil)
	c.Assert(err, gc.ErrorMatches, `unknown object type "NoSuch" \(not implemented\)`)
}
