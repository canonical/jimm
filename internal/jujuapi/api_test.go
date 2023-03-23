// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/websocket"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
)

type apiSuite struct {
	jimmtest.BootstrapSuite

	Params     jujuapi.Params
	APIHandler http.Handler
	HTTP       *httptest.Server
}

var _ = gc.Suite(&apiSuite{})

func (s *apiSuite) SetUpTest(c *gc.C) {
	s.BootstrapSuite.SetUpTest(c)
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.Handle("/api", jujuapi.APIHandler(ctx, s.JIMM, s.Params))
	mux.Handle("/model/", jujuapi.ModelHandler(ctx, s.JIMM, s.Params))
	s.APIHandler = mux
	s.HTTP = httptest.NewServer(s.APIHandler)
}

func (s *apiSuite) TearDownTest(c *gc.C) {
	if s.HTTP != nil {
		s.HTTP.Close()
		s.HTTP = nil
	}
	s.BootstrapSuite.TearDownTest(c)
}

type RedirectParams struct {
	Method         string
	URL            string
	Handler        http.Handler
	ExpectCode     int
	ExpectLocation string
}

func (s *apiSuite) TestModelCommands(c *gc.C) {
	path := fmt.Sprintf("/model/%s/commands", s.Model.UUID.String)
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

	type message struct {
		RequestID uint64                 `json:"request-id,omitempty"`
		Type      string                 `json:"type,omitempty"`
		Version   int                    `json:"version,omitempty"`
		ID        string                 `json:"id,omitempty"`
		Request   string                 `json:"request,omitempty"`
		Params    json.RawMessage        `json:"params,omitempty"`
		Error     string                 `json:"error,omitempty"`
		ErrorCode string                 `json:"error-code,omitempty"`
		ErrorInfo map[string]interface{} `json:"error-info,omitempty"`
		Response  json.RawMessage        `json:"response,omitempty"`
	}

	msg := &message{Type: "Test"}
	err = conn.WriteJSON(msg)
	// err = jsonConn.Receive(&msg)
	c.Assert(err, gc.Equals, nil)
	rcv := make(map[string]interface{})
	err = conn.ReadJSON(&rcv)
	fmt.Printf("Result: \n\n\n========%+v\n\n", rcv)
	c.Assert(err, gc.Equals, nil)
	hp := s.Model.Controller.Addresses[0][0]
	c.Assert(msg.Type, gc.Equals, fmt.Sprintf("wss://%s:%d/model/%s/commands", hp.Address.Value, hp.Port, s.Model.UUID.String))
}

func (s *apiSuite) TestModelCommandsModelNotFoundf(c *gc.C) {
	serverURL, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	u := url.URL{
		Scheme: "ws",
		Host:   serverURL.Host,
		Path:   fmt.Sprintf("/models/%s/commands", s.Model.UUID.String),
	}

	_, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.Assert(err, gc.ErrorMatches, "websocket: bad handshake")
	}
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
}
