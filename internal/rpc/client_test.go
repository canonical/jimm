// Copyright 2021 Canonical Ltd.

package rpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/rpc"
)

func TestDialError(t *testing.T) {
	c := qt.New(t)

	srv := newServer(echo)
	defer srv.Close()
	d := *srv.dialer
	d.TLSConfig = nil
	_, err := d.Dial(context.Background(), srv.URL)
	c.Assert(err, qt.ErrorMatches, `x509: certificate signed by unknown authority`)
}

func TestDial(t *testing.T) {
	c := qt.New(t)

	srv := newServer(echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
	c.Assert(err, qt.IsNil)
	defer conn.Close()
}

func TestBasicDial(t *testing.T) {
	c := qt.New(t)

	srv := newServer(echo)
	defer srv.Close()
	conn, err := srv.dialer.BasicDial(context.Background(), srv.URL)
	c.Assert(err, qt.IsNil)
	defer conn.Close()
}

func TestCallSuccess(t *testing.T) {
	c := qt.New(t)

	srv := newServer(echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
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

	srv := newServer(echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
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
		var req map[string]interface{}
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		c.Check(req["type"], qt.Equals, "test")
		c.Check(req["id"], qt.Equals, "1234")
		c.Check(req["request"], qt.Equals, "Test")
		return errors.E("test error")
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
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
		var req map[string]interface{}
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		resp := map[string]interface{}{
			"request-id": req["request-id"],
			"error":      "test error",
			"error-code": "test error code",
			"error-info": map[string]interface{}{
				"k1": "v1",
				"k2": 2,
			},
		}
		if err := conn.WriteJSON(resp); err != nil {
			return err
		}
		return echo(conn)
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "test", 0, "1234", "Test", "SUCCESS", &res)
	c.Check(err, qt.ErrorMatches, `test error \(test error code\)`)
	e := err.(*rpc.Error)
	c.Check(e.ErrorCode(), qt.Equals, "test error code")
	c.Check(e.Info, qt.DeepEquals, map[string]interface{}{
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
		var req map[string]interface{}
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		if err := conn.WriteJSON(req); err != nil {
			return err
		}
		var req2 map[string]interface{}
		if err := conn.ReadJSON(&req2); err != nil {
			return err
		}
		if err := conn.WriteJSON(req2); err != nil {
			return err
		}
		return echo(conn)
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
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
		var req map[string]interface{}
		if err := conn.ReadJSON(&req); err != nil {
			return err
		}
		if err := conn.WriteJSON(struct{}{}); err != nil {
			return err
		}
		return echo(conn)
	})
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	var res string
	err = conn.Call(context.Background(), "test", 1, "", "Test", "SUCCESS", &res)
	c.Check(err, qt.ErrorMatches, `received invalid RPC message`)
	c.Check(res, qt.Equals, "")
}

type testTokenGenerator struct{}

func (p *testTokenGenerator) MakeToken(ctx context.Context, initialLogin bool, req *params.LoginRequest, permissionMap map[string]interface{}) ([]byte, error) {
	return nil, nil
}

func (p *testTokenGenerator) SetTags(names.ModelTag, names.ControllerTag) {
}

func TestProxySockets(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	srvController := newServer(func(conn *websocket.Conn) error {
		return echo(conn)
	})
	errChan := make(chan error)
	srvJIMM := newServer(func(connClient *websocket.Conn) error {
		testTokenGen := testTokenGenerator{}
		f := func(context.Context) (*websocket.Conn, error) {
			connController, err := srvController.dialer.BasicDial(ctx, srvController.URL)
			c.Assert(err, qt.IsNil)
			return connController, nil
		}
		err := rpc.ProxySockets(ctx, connClient, &testTokenGen, f)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws, err := srvJIMM.dialer.BasicDial(ctx, srvJIMM.URL)
	c.Assert(err, qt.IsNil)
	defer ws.Close()

	p := json.RawMessage(`{"Key":"TestVal"}`)
	msg := rpc.Message{RequestID: 1, Type: "TestType", Request: "TestReq", Params: p}
	err = ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := rpc.Message{}
	err = ws.ReadJSON(&resp)
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Response, qt.DeepEquals, msg.Params)
	ws.Close()
	<-errChan // Ensure go routines are cleaned up
}

func TestCancelProxySockets(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())

	srvController := newServer(func(conn *websocket.Conn) error {
		return echo(conn)
	})

	errChan := make(chan error)
	srvJIMM := newServer(func(connClient *websocket.Conn) error {
		testTokenGen := testTokenGenerator{}
		f := func(context.Context) (*websocket.Conn, error) {
			connController, err := srvController.dialer.BasicDial(ctx, srvController.URL)
			c.Assert(err, qt.IsNil)
			return connController, nil
		}
		err := rpc.ProxySockets(ctx, connClient, &testTokenGen, f)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws, err := srvJIMM.dialer.BasicDial(ctx, srvJIMM.URL)
	c.Assert(err, qt.IsNil)
	defer ws.Close()
	cancel()
	err = <-errChan
	c.Assert(err.Error(), qt.Equals, "Context cancelled")
}

type server struct {
	*httptest.Server

	URL    string
	dialer *rpc.Dialer
}

func newServer(f func(*websocket.Conn) error) *server {
	var srv server
	srv.Server = httptest.NewTLSServer(handleWS(f))
	srv.URL = "ws" + strings.TrimPrefix(srv.Server.URL, "http")
	cp := x509.NewCertPool()
	cp.AddCert(srv.Certificate())
	srv.dialer = &rpc.Dialer{
		TLSConfig: &tls.Config{
			RootCAs: cp,
		},
	}
	return &srv
}

func handleWS(f func(*websocket.Conn) error) http.Handler {
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
		if err == nil {
			cm = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		} else if websocket.IsCloseError(err) {
			ce := err.(*websocket.CloseError)
			cm = websocket.FormatCloseMessage(ce.Code, ce.Text)
		} else {
			cm = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error())
		}
		c.WriteControl(websocket.CloseMessage, cm, time.Time{})
	})
}

func echo(c *websocket.Conn) error {
	for {
		msg := make(map[string]interface{})
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
