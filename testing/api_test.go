// Copyright 2026 Canonical.

package testing

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type apiSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&apiSuite{})

func (s *apiSuite) TestModelCommandsModelNotFoundf(c *gc.C) {
	serverURL, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	u := url.URL{
		Scheme: "wss",
		Host:   serverURL.Host,
		Path:   fmt.Sprintf("/models/%s/commands", s.Model.UUID.String),
	}
	dial := websocket.DefaultDialer
	dial.TLSClientConfig = &tls.Config{
		//nolint:gosec // we don't care about verifying test server certs
		InsecureSkipVerify: true,
	}
	_, response, err := dial.Dial(u.String(), nil)
	if err != nil {
		c.Assert(err, gc.ErrorMatches, "websocket: bad handshake")
	}
	defer response.Body.Close()

	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
}
