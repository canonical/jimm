// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"encoding/pem"
	"net/http/httptest"
	"net/url"

	"github.com/juju/juju/api"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jem/internal/apitest"
)

type websocketSuite struct {
	apitest.Suite
	wsServer   *httptest.Server
	connection api.Connection
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
	})
	defer conn.Close()
	err := conn.Login(names.NewUserTag("test-user"), "", "", nil)
	c.Assert(err, gc.ErrorMatches, `unknown model: "00000000-0000-0000-0000-000000000000" \(not found\)`)
}

func (s *websocketSuite) open(c *gc.C, info *api.Info) api.Connection {
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
	})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}
