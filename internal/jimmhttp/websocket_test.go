// Copyright 2021 Canonical Ltd.

package jimmhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jimmtest"
)

func TestWSHandler(t *testing.T) {
	c := qt.New(t)

	hnd := &jimmhttp.WSHandler{
		Server: echoServer{t: c},
	}

	srv := httptest.NewServer(hnd)
	c.Cleanup(srv.Close)

	var d websocket.Dialer
	conn, _, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	c.Assert(err, qt.IsNil)

	err = conn.WriteMessage(websocket.TextMessage, []byte("test!"))
	c.Assert(err, qt.IsNil)
	mt, p, err := conn.ReadMessage()
	c.Assert(err, qt.IsNil)
	c.Check(mt, qt.Equals, websocket.TextMessage)
	c.Check(string(p), qt.Equals, "test!")
	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	err = conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
	c.Assert(err, qt.IsNil)
}

type echoServer struct {
	t testing.TB
}

func (s echoServer) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	return ctx, nil
}

func (s echoServer) ServeWS(ctx context.Context, conn *websocket.Conn) {
	for {
		mt, p, err := conn.ReadMessage()
		if err == nil {
			err = conn.WriteMessage(mt, p)
		}
		if err != nil {
			s.t.Log(err)
			return
		}
	}
}

func TestWSHandlerPanic(t *testing.T) {
	c := qt.New(t)

	hnd := &jimmhttp.WSHandler{
		Server: panicServer{},
	}

	srv := httptest.NewServer(hnd)
	c.Cleanup(srv.Close)

	var d websocket.Dialer
	conn, _, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	c.Assert(err, qt.IsNil)

	_, _, err = conn.ReadMessage()
	c.Assert(err, qt.ErrorMatches, `websocket: close 1011 \(internal server error\): test`)
}

type panicServer struct{}

func (s panicServer) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	return ctx, nil
}

func (s panicServer) ServeWS(ctx context.Context, conn *websocket.Conn) {
	panic("test")
}

func TestWSHandlerNilServer(t *testing.T) {
	c := qt.New(t)

	hnd := &jimmhttp.WSHandler{}

	srv := httptest.NewServer(hnd)
	c.Cleanup(srv.Close)

	var d websocket.Dialer
	conn, _, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	c.Assert(err, qt.IsNil)

	_, _, err = conn.ReadMessage()
	c.Assert(err, qt.ErrorMatches, `websocket: close 1000 \(normal\)`)
}

type authFailServer struct{ c jimmtest.SimpleTester }

func (s authFailServer) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	return ctx, errors.E("authentication failed")
}

func (s authFailServer) ServeWS(ctx context.Context, conn *websocket.Conn) {}

func TestWSHandlerAuthFailsServer(t *testing.T) {
	c := qt.New(t)

	hnd := &jimmhttp.WSHandler{
		Server: authFailServer{c: c},
	}

	srv := httptest.NewServer(hnd)
	c.Cleanup(srv.Close)

	var d websocket.Dialer
	conn, _, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), http.Header{
		"Cookie": []string{auth.SessionName + "=naughty_cookie"},
	})
	c.Assert(err, qt.IsNil)

	_, _, err = conn.ReadMessage()
	c.Assert(err, qt.ErrorMatches, `websocket: close 1011 \(internal server error\): authentication failed`)
}
