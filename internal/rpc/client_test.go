// Copyright 2025 Canonical.

package rpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/rpc"
	"github.com/canonical/jimm/v3/internal/testutils/rpctest"
)

func TestDialError(t *testing.T) {
	c := qt.New(t)

	srv := newServer(rpctest.Echo)
	defer srv.Close()
	d := *srv.dialer
	d.TLSConfig = nil
	_, err := d.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.ErrorMatches, `.*x509: certificate signed by unknown authority`)
}

func TestDial(t *testing.T) {
	c := qt.New(t)

	srv := newServer(rpctest.Echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()
}

func TestBasicDial(t *testing.T) {
	c := qt.New(t)

	srv := newServer(rpctest.Echo)
	defer srv.Close()
	conn, err := srv.dialer.DialWebsocket(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()
}

func TestCallSuccess(t *testing.T) {
	c := qt.New(t)

	srv := newServer(rpctest.Echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "Test", 1, "", "Test", "SUCCESS", &res)
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.Equals, "SUCCESS")
	err = conn.Call(context.Background(), "Test", 1, "", "Test", "SUCCESS AGAIN", &res)
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.Equals, "SUCCESS AGAIN")
}

func TestCallCanceledContext(t *testing.T) {
	c := qt.New(t)

	srv := newServer(rpctest.Echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = conn.Call(ctx, "Test", 1, "", "Test", "SUCCESS", &res)
	c.Assert(err, qt.ErrorMatches, "context canceled")
	c.Check(res, qt.Equals, "")
	err = conn.Call(context.Background(), "Test", 1, "", "Test", "SUCCESS", &res)
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.Equals, "SUCCESS")
}

func TestCallClosedWithoutResponse(t *testing.T) {
	c := qt.New(t)

	srv := newServer(func(conn *websocket.Conn) error {
		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		c.Check(req["type"], qt.Equals, "test")
		c.Check(req["id"], qt.Equals, "1234")
		c.Check(req["request"], qt.Equals, "Test")
		return errors.New("test error")
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "test", 0, "1234", "Test", "SUCCESS", &res)
	c.Assert(err, qt.ErrorMatches, `websocket: close 1011 \(internal server error\): test error`)
	c.Check(res, qt.Equals, "")
}

func TestCallErrorResponse(t *testing.T) {
	c := qt.New(t)

	srv := newServer(func(conn *websocket.Conn) error {
		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		resp := map[string]any{
			"request-id": req["request-id"],
			"error":      "test error",
			"error-code": "test error code",
			"error-info": map[string]any{
				"k1": "v1",
				"k2": 2,
			},
		}
		if err := conn.WriteJSON(resp); err != nil {
			return err
		}
		return rpctest.Echo(conn)
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "test", 0, "1234", "Test", "SUCCESS", &res)
	c.Check(err, qt.ErrorMatches, `test error \(test error code\)`)
	e, ok := err.(*rpc.Error)
	c.Logf("expected %T, received %T", e, err)
	c.Assert(ok, qt.IsTrue)

	c.Check(e.ErrorCode(), qt.Equals, "test error code")
	c.Check(e.Info, qt.DeepEquals, map[string]any{
		"k1": "v1",
		"k2": float64(2),
	})
	c.Check(res, qt.Equals, "")

	err = conn.Call(context.Background(), "test", 1, "", "Test", "SUCCESS", &res)
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.Equals, "SUCCESS")
}

func TestClientReceiveRequest(t *testing.T) {
	c := qt.New(t)

	srv := newServer(func(conn *websocket.Conn) error {
		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		if err := conn.WriteJSON(req); err != nil {
			return err
		}
		var req2 map[string]any
		if err := conn.ReadJSON(&req2); err != nil {
			return err
		}
		if err := conn.WriteJSON(req2); err != nil {
			return err
		}
		return rpctest.Echo(conn)
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "test", 1, "", "Test", "SUCCESS", &res)
	c.Check(err, qt.ErrorMatches, `test\(1\).Test not implemented \(not implemented\)`)
	e := err.(*rpc.Error)
	c.Check(e.ErrorCode(), qt.Equals, "not implemented")
	c.Check(res, qt.Equals, "")

	err = conn.Call(context.Background(), "test", 1, "", "Test", "SUCCESS", &res)
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.Equals, "SUCCESS")
}

func TestClientReceiveInvalidMessage(t *testing.T) {
	c := qt.New(t)

	srv := newServer(func(conn *websocket.Conn) error {
		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		if err := conn.WriteJSON(struct{}{}); err != nil {
			return err
		}
		return rpctest.Echo(conn)
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL, nil)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "test", 1, "", "Test", "SUCCESS", &res)
	c.Check(err, qt.ErrorMatches, `received invalid RPC message`)
	c.Check(res, qt.Equals, "")
}

type server struct {
	*httptest.Server

	URL    string
	dialer *rpc.Dialer
}

func newServer(f func(*websocket.Conn) error) *server {
	var srv server
	srv.Server = httptest.NewTLSServer(rpctest.HandleWS(f))
	srv.URL = "ws" + strings.TrimPrefix(srv.Server.URL, "http")
	cp := x509.NewCertPool()
	cp.AddCert(srv.Certificate())
	srv.dialer = &rpc.Dialer{
		TLSConfig: &tls.Config{
			RootCAs:    cp,
			MinVersion: tls.VersionTLS12,
		},
	}
	return &srv
}

func newIPv6Server(f func(*websocket.Conn) error) *server {
	var srv server
	l, _ := net.Listen("tcp", "[::1]:0")
	server := httptest.Server{
		Listener: l,
		Config:   &http.Server{Handler: rpctest.HandleWS(f)}, //nolint:gosec
	}
	server.StartTLS()
	srv.Server = &server
	srv.URL = "ws" + strings.TrimPrefix(srv.Server.URL, "http")
	cp := x509.NewCertPool()
	cp.AddCert(srv.Certificate())
	srv.dialer = &rpc.Dialer{
		TLSConfig: &tls.Config{
			RootCAs:    cp,
			MinVersion: tls.VersionTLS12,
		},
	}
	return &srv
}
