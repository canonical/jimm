// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/testing/httptesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/jemtest/apitest"
	"github.com/canonical/jimm/internal/jujuapi"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type apiSuite struct {
	apitest.BootstrapAPISuite
}

var _ = gc.Suite(&apiSuite{})

func (s *apiSuite) SetUpTest(c *gc.C) {
	s.NewAPIHandler = jujuapi.NewAPIHandler
	s.Params.GUILocation = "https://jujucharms.com.test"
	s.BootstrapAPISuite.SetUpTest(c)
}

func (s *apiSuite) TestGUI(c *gc.C) {
	AssertRedirect(c, RedirectParams{
		Handler:        s.APIHandler,
		Method:         "GET",
		URL:            fmt.Sprintf("/gui/%s", s.Model.UUID),
		ExpectCode:     http.StatusMovedPermanently,
		ExpectLocation: "https://jujucharms.com.test/u/bob/model-1",
	})
}

func (s *apiSuite) TestGUINotFound(c *gc.C) {
	p := s.Params
	p.GUILocation = ""
	hnd := s.NewAPIHTTPHandler(c, p)
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		URL:          fmt.Sprintf("/gui/%s", "000000000000-0000-0000-0000-00000000"),
		Handler:      hnd,
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Code:    params.ErrNotFound,
			Message: "no GUI location specified",
		},
	})
}

func (s *apiSuite) TestGUIModelNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		URL:          fmt.Sprintf("/gui/%s", "000000000000-0000-0000-0000-00000000"),
		Handler:      s.APIHandler,
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Code:    params.ErrNotFound,
			Message: `model not found`,
		},
	})
}

type RedirectParams struct {
	Method         string
	URL            string
	Handler        http.Handler
	ExpectCode     int
	ExpectLocation string
}

// AssertRedirect checks that a handler returns a redirect
func AssertRedirect(c *gc.C, p RedirectParams) {
	if p.Method == "" {
		p.Method = "GET"
	}
	req, err := http.NewRequest(p.Method, p.URL, nil)
	c.Assert(err, gc.Equals, nil)
	rr := httptest.NewRecorder()
	p.Handler.ServeHTTP(rr, req)
	if p.ExpectCode == 0 {
		c.Assert(300 <= rr.Code && rr.Code < 400, gc.Equals, true, gc.Commentf("Expected redirect status (3XX), got %d", rr.Code))
	} else {
		c.Assert(rr.Code, gc.Equals, p.ExpectCode)
	}
	c.Assert(rr.HeaderMap.Get("Location"), gc.Equals, p.ExpectLocation)
}

func (s *apiSuite) TestModelCommands(c *gc.C) {
	path := fmt.Sprintf("/model/%s/commands", s.Model.UUID)
	serverURL, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	u := url.URL{
		Scheme: "ws",
		Host:   serverURL.Host,
		Path:   path,
	}

	conn, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.Assert(err, gc.Equals, nil)
	}
	c.Assert(response.StatusCode, gc.Equals, http.StatusSwitchingProtocols)
	defer conn.Close()

	msg := struct {
		RedirectTo string `json:"redirect-to"`
	}{}
	jsonConn := jsoncodec.NewWebsocketConn(conn)
	err = jsonConn.Receive(&msg)
	c.Assert(err, gc.Equals, nil)
	c.Assert(msg.RedirectTo, gc.Equals, fmt.Sprintf("wss://%s/model/%s/commands", mongodoc.Addresses(s.Controller.HostPorts)[0], s.Model.UUID))
}

func (s *apiSuite) TestModelCommandsModelNotFoundf(c *gc.C) {
	serverURL, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	u := url.URL{
		Scheme: "ws",
		Host:   serverURL.Host,
		Path:   fmt.Sprintf("/models/%s/commands", s.Model.UUID),
	}

	_, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.Assert(err, gc.ErrorMatches, "websocket: bad handshake")
	}
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
}
