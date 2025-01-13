// Copyright 2025 Canonical.

package streamproxy_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/v3/internal/streamproxy"
	"github.com/canonical/jimm/v3/internal/testutils/rpctest"
)

func echoSingleMessage(c *websocket.Conn) error {
	msg := make(map[string]interface{})
	if err := c.ReadJSON(&msg); err != nil {
		return err
	}
	if err := c.WriteJSON(msg); err != nil {
		return err
	}
	return nil
}

func verifyEcho(c *qt.C, ws *websocket.Conn, expectedErr string) {
	msg := json.RawMessage(`{"Key":"TestVal"}`)
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := json.RawMessage{}
	receiveChan := make(chan error)
	go func() {
		receiveChan <- ws.ReadJSON(&resp)
	}()
	select {
	case err := <-receiveChan:
		if expectedErr == "" {
			c.Assert(err, qt.IsNil)
		} else {
			c.Assert(err, qt.ErrorMatches, expectedErr)
			return
		}
	case <-time.After(5 * time.Second):
		c.Logf("took too long to read response")
		c.FailNow()
	}
	c.Assert(string(resp), qt.DeepEquals, string(msg))
}

func TestStreamProxy(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	doneChan := make(chan error)
	srvController := rpctest.NewServer(echoSingleMessage)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
		streamproxy.ProxyStreams(ctx, connClient, connController)
		doneChan <- nil
		return nil
	})
	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	verifyEcho(c, ws, "")

	ws.Close()
	<-doneChan // Ensure go routines are cleaned up
}

func TestStreamProxyStoppedController(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	doneChan := make(chan error)
	srvController := rpctest.NewServer(func(c *websocket.Conn) error { return errors.New("stopped") })
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
		streamproxy.ProxyStreams(ctx, connClient, connController)
		doneChan <- nil
		return nil
	})
	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	verifyEcho(c, ws, ".*abnormal closure.*")

	ws.Close()
	<-doneChan // Ensure go routines are cleaned up
}

func TestStreamProxyStoppedMidwayController(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	doneChan := make(chan error)
	srvController := rpctest.NewServer(echoSingleMessage)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
		streamproxy.ProxyStreams(ctx, connClient, connController)
		doneChan <- nil
		return nil
	})
	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	verifyEcho(c, ws, "")
	verifyEcho(c, ws, ".*abnormal closure.*")

	ws.Close()
	<-doneChan // Ensure go routines are cleaned up
}
