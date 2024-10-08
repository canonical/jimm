// Copyright 2024 Canonical.
package rpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/canonical/jimm/v3/internal/rpc"
)

func streamEcho(c *websocket.Conn, stopped *bool) error {
	for {
		msg := make(map[string]interface{})
		if *stopped {
			return errors.New("stopped")
		}
		if err := c.ReadJSON(&msg); err != nil {
			return err
		}
		if err := c.WriteJSON(msg); err != nil {
			return err
		}
	}
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
	stopped := false
	srvController := newServer(func(c *websocket.Conn) error { return streamEcho(c, &stopped) })
	srvJIMM := newServer(func(connClient *websocket.Conn) error {
		connController, err := srvController.dialer.DialWebsocket(ctx, srvController.URL, nil)
		c.Assert(err, qt.IsNil)
		rpc.ProxyStreams(ctx, connClient, connController)
		doneChan <- nil
		return nil
	})
	defer srvController.Close()
	defer srvJIMM.Close()
	ws, err := srvJIMM.dialer.DialWebsocket(ctx, srvJIMM.URL, nil)
	c.Assert(err, qt.IsNil)
	defer ws.Close()

	verifyEcho(c, ws, "")

	ws.Close()
	<-doneChan // Ensure go routines are cleaned up
}

func TestStreamProxyStoppedController(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	doneChan := make(chan error)
	stopped := false
	srvController := newServer(func(c *websocket.Conn) error { return streamEcho(c, &stopped) })
	srvJIMM := newServer(func(connClient *websocket.Conn) error {
		connController, err := srvController.dialer.DialWebsocket(ctx, srvController.URL, nil)
		c.Assert(err, qt.IsNil)
		rpc.ProxyStreams(ctx, connClient, connController)
		doneChan <- nil
		return nil
	})
	defer srvController.Close()
	defer srvJIMM.Close()
	ws, err := srvJIMM.dialer.DialWebsocket(ctx, srvJIMM.URL, nil)
	c.Assert(err, qt.IsNil)
	defer ws.Close()

	stopped = true
	verifyEcho(c, ws, ".*abnormal closure.*")

	ws.Close()
	<-doneChan // Ensure go routines are cleaned up
}
