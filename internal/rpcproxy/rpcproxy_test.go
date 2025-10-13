// Copyright 2025 Canonical.

package rpcproxy_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpcproxy"
	"github.com/canonical/jimm/v3/internal/testutils/rpctest"
)

type testTokenGenerator struct{}

func (p *testTokenGenerator) MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error) {
	return nil, nil
}

func (p *testTokenGenerator) MakeToken(ctx context.Context, permissionMap map[string]interface{}) ([]byte, error) {
	return nil, nil
}

func (p *testTokenGenerator) SetTags(names.ModelTag, names.ControllerTag) {
}

func (p *testTokenGenerator) GetUser() names.UserTag {
	return names.NewUserTag("testUser")
}

func TestProxySockets(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	srvController := rpctest.NewServer(rpctest.Echo)

	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		testTokenGen := testTokenGenerator{}
		f := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:      connController,
				ModelName: "TestName",
			}, nil
		}
		auditLogger := func(ale *dbmodel.AuditLogEntry) {}
		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGen,
			ConnectController: f,
			AuditLog:          auditLogger,
			LoginService:      &mockLoginService{},
		}
		err := rpcproxy.ProxySockets(ctx, proxyHelpers)
		c.Check(err, qt.IsNil)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	p := json.RawMessage(`{"Key":"TestVal"}`)
	msg := rpcproxy.Message{RequestID: 1, Type: "TestType", Request: "TestReq", Params: p}
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := rpcproxy.Message{}
	receiveChan := make(chan error)
	go func() {
		receiveChan <- ws.ReadJSON(&resp)
	}()
	select {
	case err := <-receiveChan:
		c.Assert(err, qt.IsNil)
	case <-time.After(5 * time.Second):
		c.Logf("took too long to read response")
		c.FailNow()
	case err := <-errChan:
		c.Fatal(err)
	}
	c.Assert(resp.Response, qt.DeepEquals, msg.Params)
	ws.Close()
	<-errChan // Ensure go routines are cleaned up
}

func TestProxySocketsControllerConnectionFails(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	srvController := rpctest.NewServer(rpctest.Echo)

	var connController *websocket.Conn
	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		testTokenGen := testTokenGenerator{}
		f := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			var err error
			connController = srvController.Dialer.DialWebsocket(c, srvController.URL)
			c.Check(err, qt.IsNil)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:      connController,
				ModelName: "TestName",
			}, nil
		}
		auditLogger := func(ale *dbmodel.AuditLogEntry) {}
		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGen,
			ConnectController: f,
			AuditLog:          auditLogger,
			LoginService:      &mockLoginService{},
		}
		err := rpcproxy.ProxySockets(ctx, proxyHelpers)
		c.Check(err, qt.IsNil)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	p := json.RawMessage(`{"Key":"TestVal"}`)
	msg := rpcproxy.Message{RequestID: 1, Type: "TestType", Request: "TestReq", Params: p}
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := rpcproxy.Message{}
	receiveChan := make(chan error)
	go func() {
		receiveChan <- ws.ReadJSON(&resp)
	}()
	select {
	case err := <-receiveChan:
		c.Assert(err, qt.IsNil)
	case <-time.After(5 * time.Second):
		c.Logf("took too long to read response")
		c.FailNow()
	case err := <-errChan:
		c.Fatal(err)
	}
	c.Assert(resp.Response, qt.DeepEquals, msg.Params)

	// Now close the connection to the controller and ensure the model proxy is cleaned up.
	connController.Close()
	select {
	case <-time.After(5 * time.Second):
		c.Fatalf("model proxy did not return in time")
	case <-errChan: // Ensure go routines are cleaned up
	}
}

func TestCancelProxySockets(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())

	srvController := rpctest.NewServer(rpctest.Echo)

	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		testTokenGen := testTokenGenerator{}
		f := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:      connController,
				ModelName: "TestName",
			}, nil
		}
		auditLogger := func(ale *dbmodel.AuditLogEntry) {}
		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGen,
			ConnectController: f,
			AuditLog:          auditLogger,
			LoginService:      &mockLoginService{},
		}
		err := rpcproxy.ProxySockets(ctx, proxyHelpers)
		c.Check(err, qt.ErrorMatches, "Context cancelled")
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()
	cancel()
	select {
	case <-time.After(5 * time.Second):
		c.Fatalf("model proxy did not return in time")
	case <-errChan: // Ensure go routines are cleaned up
	}
}

func TestProxySocketsAuditLogs(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	srvController := rpctest.NewServer(rpctest.Echo)
	auditLogs := make([]*dbmodel.AuditLogEntry, 0)

	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		defer connClient.Close()
		testTokenGen := testTokenGenerator{}
		f := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:      connController,
				ModelName: "TestModelName",
			}, nil
		}
		auditLogger := func(ale *dbmodel.AuditLogEntry) { auditLogs = append(auditLogs, ale) }
		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGen,
			ConnectController: f,
			AuditLog:          auditLogger,
			LoginService:      &mockLoginService{},
		}
		err := rpcproxy.ProxySockets(ctx, proxyHelpers)
		c.Check(err, qt.IsNil)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	p := json.RawMessage(`{"Key":"TestVal"}`)
	msg := rpcproxy.Message{RequestID: 1, Type: "TestType", Request: "TestReq", Params: p}
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := rpcproxy.Message{}
	err = ws.ReadJSON(&resp)
	c.Assert(err, qt.IsNil)
	ws.Close()

	select {
	case <-time.After(5 * time.Second):
		c.Fatalf("model proxy did not return in time")
	case <-errChan: // Ensure go routines are cleaned up
	}

	c.Assert(auditLogs, qt.HasLen, 2)
	expectedEvents := []*dbmodel.AuditLogEntry{{
		ID:             auditLogs[0].ID,
		Time:           auditLogs[0].Time,
		Model:          "TestModelName",
		ConversationId: auditLogs[0].ConversationId,
		MessageId:      1,
		FacadeName:     "TestType",
		FacadeMethod:   "TestReq",
		FacadeVersion:  0,
		ObjectId:       "",
		IdentityTag:    "user-testUser",
		IsResponse:     false,
		Params:         dbmodel.JSON(p),
		Errors:         nil,
	}, {
		ID:             auditLogs[1].ID,
		Time:           auditLogs[1].Time,
		Model:          "TestModelName",
		ConversationId: auditLogs[1].ConversationId,
		MessageId:      1,
		FacadeName:     "",
		FacadeMethod:   "",
		FacadeVersion:  0,
		ObjectId:       "",
		IdentityTag:    "user-testUser",
		IsResponse:     true,
		Params:         nil,
		Errors:         auditLogs[1].Errors,
	},
	}
	c.Assert(auditLogs, qt.DeepEquals, expectedEvents)
}

// TestProxySocketsModelMigration verifies that when a model is migrating
// and we get back and unauthorized error from the controller, we modify
// the error to indicate that the model is completing migration.
func TestProxySocketsModelMigration(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	srvController := rpctest.NewServer(rpctest.Unauthorized)

	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		f := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:          connController,
				ModelName:     "TestName",
				MigrationMode: dbmodel.MigrationModeMigrateInternal,
			}, nil
		}

		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGenerator{},
			ConnectController: f,
			AuditLog:          func(ale *dbmodel.AuditLogEntry) {},
			LoginService:      &mockLoginService{},
		}
		err := rpcproxy.ProxySockets(ctx, proxyHelpers)
		c.Check(err, qt.IsNil)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	msg := rpcproxy.Message{RequestID: 1, Type: "TestType", Request: "TestReq"}
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)

	resp := rpcproxy.Message{}
	receiveChan := make(chan error)
	go func() {
		receiveChan <- ws.ReadJSON(&resp)
	}()
	select {
	case err := <-receiveChan:
		c.Assert(err, qt.IsNil)
	case <-time.After(5 * time.Second):
		c.Logf("took too long to read response")
		c.FailNow()
	case err := <-errChan:
		c.Fatal(err)
	}

	c.Assert(resp.Error, qt.Equals, "model is finishing migration, please retry later")
	c.Assert(resp.ErrorCode, qt.Equals, string(errors.CodeModelMigrating))
	ws.Close()
	<-errChan // Ensure go routines are cleaned up
}

// TestProxySocketsUnauthorized complements TestProxySocketsModelMigration
// by verifying that if the model is not migrating, we do not modify the
// unauthorized error from the controller.
func TestProxySocketsUnauthorized(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	srvController := rpctest.NewServer(rpctest.Unauthorized)

	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		f := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:      connController,
				ModelName: "TestName",
			}, nil
		}

		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGenerator{},
			ConnectController: f,
			AuditLog:          func(ale *dbmodel.AuditLogEntry) {},
			LoginService:      &mockLoginService{},
		}
		err := rpcproxy.ProxySockets(ctx, proxyHelpers)
		c.Check(err, qt.IsNil)
		errChan <- err
		return err
	})

	defer srvController.Close()
	defer srvJIMM.Close()
	ws := srvJIMM.Dialer.DialWebsocket(c, srvJIMM.URL)
	defer ws.Close()

	msg := rpcproxy.Message{RequestID: 1, Type: "TestType", Request: "TestReq"}
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)

	resp := rpcproxy.Message{}
	receiveChan := make(chan error)
	go func() {
		receiveChan <- ws.ReadJSON(&resp)
	}()
	select {
	case err := <-receiveChan:
		c.Assert(err, qt.IsNil)
	case <-time.After(5 * time.Second):
		c.Logf("took too long to read response")
		c.FailNow()
	case err := <-errChan:
		c.Fatal(err)
	}

	c.Assert(resp.Error, qt.Equals, "unauthorized access error message from controller")
	c.Assert(resp.ErrorCode, qt.Equals, string(errors.CodeUnauthorized))
	ws.Close()
	<-errChan // Ensure go routines are cleaned up
}
