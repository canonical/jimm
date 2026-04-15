// Copyright 2025 Canonical.

package rpctest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/v3/internal/errors"
)

type testDialer struct {
	tlsConfig *tls.Config
}

// Dial establishes a new client RPC connection to the given URL.
func (d *testDialer) DialWebsocket(c *qt.C, url string) *websocket.Conn {
	dialer := websocket.Dialer{
		TLSClientConfig: d.tlsConfig,
	}
	conn, resp, err := dialer.DialContext(context.Background(), url, nil)
	c.Assert(err, qt.IsNil)
	defer resp.Body.Close()
	return conn
}

type Server struct {
	*httptest.Server

	URL    string
	Dialer *testDialer
}

func NewServer(f func(*websocket.Conn) error) *Server {
	var srv Server
	srv.Server = httptest.NewTLSServer(HandleWS(f))
	srv.URL = "ws" + strings.TrimPrefix(srv.Server.URL, "http")
	cp := x509.NewCertPool()
	cp.AddCert(srv.Certificate())
	srv.Dialer = &testDialer{
		tlsConfig: &tls.Config{
			RootCAs:    cp,
			MinVersion: tls.VersionTLS12,
		},
	}
	return &srv
}

func HandleWS(f func(*websocket.Conn) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var u websocket.Upgrader
		c, err := u.Upgrade(w, req, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer c.Close()
		err = f(c)
		var cm []byte
		closeError, isCloseError := err.(*websocket.CloseError)
		switch {
		case err == nil:
			cm = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		case isCloseError:
			cm = websocket.FormatCloseMessage(closeError.Code, closeError.Text)
		default:
			cm = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error())
		}
		_ = c.WriteControl(websocket.CloseMessage, cm, time.Time{})

	})
}

func Echo(c *websocket.Conn) error {
	for {
		msg := make(map[string]any)
		if err := c.ReadJSON(&msg); err != nil {
			return err
		}
		delete(msg, "type")
		delete(msg, "version")
		delete(msg, "id")
		delete(msg, "request")
		msg["response"] = msg["params"]
		delete(msg, "params")
		if err := c.WriteJSON(msg); err != nil {
			return err
		}
	}
}

func Unauthorized(c *websocket.Conn) error {
	for {
		msg := make(map[string]any)
		if err := c.ReadJSON(&msg); err != nil {
			return err
		}
		msg["error"] = "unauthorized access error message from controller"
		msg["error-code"] = errors.CodeUnauthorized
		msg["response"] = map[string]any{}
		if err := c.WriteJSON(msg); err != nil {
			return err
		}
	}
}
