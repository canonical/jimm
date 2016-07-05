// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"encoding/pem"
	"net/http/httptest"
	"net/url"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/params"
)

type websocketSuite struct {
	apitest.Suite
	wsServer *httptest.Server
}

var _ = gc.Suite(&websocketSuite{})

func (s *websocketSuite) SetUpTest(c *gc.C) {
	s.Suite.SetUpTest(c)
	s.wsServer = httptest.NewTLSServer(s.JEMSrv)
}

func (s *websocketSuite) TearDownTest(c *gc.C) {
	s.wsServer.Close()
	s.Suite.TearDownTest(c)
}

func (s *websocketSuite) TestUnknownModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag("00000000-0000-0000-0000-000000000000"),
		SkipLogin: true,
	}, "bob")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.ErrorMatches, `model "00000000-0000-0000-0000-000000000000" not found \(not found\)`)
}

func (s *websocketSuite) TestLoginToModel(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(jujuparams.IsRedirect(err), gc.Equals, true)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, jc.ErrorIsNil)
	nhps, err := network.ParseHostPorts(s.APIInfo(c).Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	hps := jujuparams.FromNetworkHostPorts(nhps)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{
		Servers: [][]jujuparams.HostPort{hps},
		CACert:  s.APIInfo(c).CACert,
	})
}

func (s *websocketSuite) TestIncorrectUserFails(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "bob")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *websocketSuite) TestRedirectInfoFailsWithoutLogin(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *websocketSuite) TestOldAdminVersionFails(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JAAS does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *websocketSuite) TestAdminIDFails(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "Object ID", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "id not found")
}

func (s *websocketSuite) TestLoginToController(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	conn := s.open(c, &api.Info{
		ModelTag:  s.APIInfo(c).ModelTag,
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "not redirected")
}

func (s *websocketSuite) TestUnimplementedMethodFails(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "Logout", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.Logout is not implemented \(not implemented\)`)
}

func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	inf := *info
	u, err := url.Parse(s.wsServer.URL)
	c.Assert(err, jc.ErrorIsNil)
	inf.Addrs = []string{
		u.Host,
	}
	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.wsServer.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, jc.ErrorIsNil)
	inf.CACert = w.String()
	conn, err := api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       s.IDMSrv.Client(username),
	})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}
