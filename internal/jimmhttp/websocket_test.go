// Copyright 2021 Canonical Ltd.

package jimmhttp_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/internal/jimmhttp"
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
