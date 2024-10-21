// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/websocket"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
	mux.Handle("/model/", http.StripPrefix("/model", jujuapi.ModelHandler(ctx, s.JIMM, s.Params)))
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
	defer response.Body.Close()

	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
}
