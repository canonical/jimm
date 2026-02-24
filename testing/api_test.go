// Copyright 2025 Canonical.

package testing

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestModelCommandsModelNotFoundf(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	serverURL, err := url.Parse(s.HTTP.URL)
	c.Assert(err, qt.Equals, nil)
	u := url.URL{
		Scheme: "wss",
		Host:   serverURL.Host,
		Path:   fmt.Sprintf("/models/%s/commands", model.UUID.String),
	}
	dial := websocket.DefaultDialer
	dial.TLSClientConfig = &tls.Config{
		//nolint:gosec // we don't care about verifying test server certs
		InsecureSkipVerify: true,
	}
	_, response, err := dial.Dial(u.String(), nil)
	if err != nil {
		c.Assert(err, qt.ErrorMatches, "websocket: bad handshake")
	}
	defer response.Body.Close()

	c.Assert(response.StatusCode, qt.Equals, http.StatusNotFound)
}
