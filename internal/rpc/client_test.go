// Copyright 2021 Canonical Ltd.

package rpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

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
	c.Assert(err, qt.ErrorMatches, `tls: failed to verify certificate: x509: certificate signed by unknown authority`)
}

func TestDial(t *testing.T) {
	c := qt.New(t)

	srv := newServer(echo)
	defer srv.Close()
	conn, err := srv.dialer.Dial(context.Background(), srv.URL)
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

func TestAlexDiglett(t *testing.T) {
	c := qt.New(t)

	controller := server{}

	controller.Server = httptest.NewTLSServer(
		handleWS(
			func(connClient *websocket.Conn) error {
				return echo(connClient)
			},
		),
	)
	controller.URL = "ws" + strings.TrimPrefix(controller.Server.URL, "http")
	cp := x509.NewCertPool()
	cp.AddCert(controller.Certificate())
	controller.dialer = &rpc.Dialer{
		TLSConfig: &tls.Config{
			RootCAs: cp,
		},
	}

	controllerClient, err := controller.dialer.Dial(context.Background(), controller.URL)
	c.Assert(err, qt.IsNil)

	srvJIMM := newServer(func(connClient *websocket.Conn) error {
		return rpc.ProxySockets(context.Background(), connClient, controllerClient.GetConn())
	})

	srvClient, _ := srvJIMM.dialer.Dial(context.Background(), srvJIMM.URL)
	msg := rpc.Message{RequestID: 1, Type: "TestType", Request: "TestReq"}
	srvClient.GetConn().WriteJSON(msg)
	srvClient.GetConn().ReadMessage()

}

func TestProxySockets(t *testing.T) {
	c := qt.New(t)

	ctx := context.TODO()

	srvController := newServer(func(conn *websocket.Conn) error {
		return echo(conn)
	})

	srvJIMM := newServer(func(connClient *websocket.Conn) error {
		controllerClient, err := srvController.dialer.Dial(ctx, srvController.URL)
		c.Assert(err, qt.IsNil)
		defer controllerClient.Close()
		connController := controllerClient.GetConn()
		err = rpc.ProxySockets(ctx, connClient, connController)
		fmt.Println("Proxy error -", err)
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	client, err := srvJIMM.dialer.Dial(ctx, srvJIMM.URL)
	c.Assert(err, qt.IsNil)
	defer client.Close()

	ws := client.GetConn()
	p := json.RawMessage(`{"Key": "TestVal"}`)
	msg := rpc.Message{RequestID: 1, Type: "TestType", Request: "TestReq", Params: p}
	fmt.Printf("Writing message: %+v\n", msg)
	err = ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := rpc.Message{}
	fmt.Printf("Reading response\n")
	err = ws.ReadJSON(&resp)
	fmt.Printf("Got response: %+v\n", resp)
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Params, qt.Equals, p)
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
		fmt.Printf("Echo server awaiting msg.\n")
		if err := c.ReadJSON(&msg); err != nil {
			return err
		}
		fmt.Printf("Received %+v on echo server\n", msg)
		delete(msg, "type")
		delete(msg, "version")
		delete(msg, "id")
		delete(msg, "request")
		msg["response"] = msg["params"]
		delete(msg, "params")
		fmt.Printf("Writing %+v on echo server\n", msg)
		diglett := make(map[string]interface{})
		// diglett["request-id"] = 10
		diglett["version"] = 10
		// diglett["response"] = msg["response"]
		diglett["name"] = "diglett"
		//{"name":"diglett","response":"diglett"}
		blah, err := json.Marshal(diglett)
		if err != nil {
			fmt.Println("Marshal err", err)
		}
		fmt.Printf("Sending - %s\n", blah)
		err = c.WriteMessage(1, blah)
		if err != nil {
			fmt.Println("Write message err", err)
		}
		// if err := c.WriteJSON(msg); err != nil {
		// 	fmt.Printf("Error writing in echo service - %s\n", err.Error())
		// 	return err
		// }
		fmt.Printf("Writing msg echo server done.\n")
	}
}
