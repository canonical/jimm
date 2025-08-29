// Copyright 2025 Canonical.

package rpcproxy_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpcproxy"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
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
			SSHKeyManager:     &mocks.SSHKeyManager{},
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
			SSHKeyManager:     &mocks.SSHKeyManager{},
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
			SSHKeyManager:     &mocks.SSHKeyManager{},
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
			SSHKeyManager:     &mocks.SSHKeyManager{},
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
			SSHKeyManager:     &mocks.SSHKeyManager{},
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
			SSHKeyManager:     &mocks.SSHKeyManager{},
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

// TestProxySocketsSSHKeys verifies that the
// SSH KeyManager methods are wired up properly.
// E.g. that a call with method name AddKeys calls
// the KeyManager's AddKeys method.
func TestProxySocketsSSHKeys(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	sshFacadeChan := make(chan (string), 1)

	srvController := rpctest.NewServer(rpctest.Echo)

	errChan := make(chan error)
	srvJIMM := rpctest.NewServer(func(connClient *websocket.Conn) error {
		defer connClient.Close()
		testTokenGen := testTokenGenerator{}
		connectControllerF := func(context.Context) (rpcproxy.WebsocketConnectionWithMetadata, error) {
			connController := srvController.Dialer.DialWebsocket(c, srvController.URL)
			return rpcproxy.WebsocketConnectionWithMetadata{
				Conn:      connController,
				ModelName: "TestModelName",
			}, nil
		}
		proxyHelpers := rpcproxy.ProxyHelpers{
			RedirectInfo:      &mockRedirectInfo{},
			ConnClient:        connClient,
			TokenGen:          &testTokenGen,
			ConnectController: connectControllerF,
			AuditLog:          func(ale *dbmodel.AuditLogEntry) {},
			LoginService: &mockLoginService{
				email: "alice@canonical.com",
			},
			SSHKeyManager: &mocks.SSHKeyManager{
				AddUserPublicKey_: func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey sshkeys.PublicKey) error {
					sshFacadeChan <- "add-keys"
					return nil
				},
				ListUserPublicKeys_: func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error) {
					sshFacadeChan <- "list-keys"
					return nil, nil
				},
				RemoveUserKeyByComment_: func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, comment string) error {
					sshFacadeChan <- "remove-keys-comment"
					return nil
				},
				RemoveUserKeyByFingerprint_: func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, fingerprint string) error {
					sshFacadeChan <- "remove-keys-fingerprint"
					return nil
				},
				VerifyPublicKey_: func(ctx context.Context, claimUser string, publicKey []byte) (bool, error) {
					sshFacadeChan <- "verify-key"
					return true, nil
				},
			},
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

	// Perform login
	p := json.RawMessage(`{"Key":"TestVal"}`)
	msg := rpcproxy.Message{RequestID: 1, Type: "Admin", Request: "LoginWithSessionToken", Params: p} // #nosec G115 accept integer conversion
	err := ws.WriteJSON(&msg)
	c.Assert(err, qt.IsNil)
	resp := rpcproxy.Message{}
	err = ws.ReadJSON(&resp)
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Error, qt.Equals, "")

	// Run sub-tests for all SSH Key methods
	tests := []struct {
		name               string
		request            string
		params             []byte
		expectedChanResult string
		expectedErr        string
	}{
		{
			name:               "Add key method",
			request:            "AddKeys",
			expectedChanResult: "add-keys",
			params:             mustMarshal(jujuparams.ModifyUserSSHKeys{Keys: []string{"type key comment"}}),
		},
		{
			name:               "List keys method",
			request:            "ListKeys",
			expectedChanResult: "list-keys",
			params:             mustMarshal(jujuparams.ListSSHKeys{Mode: ssh.Fingerprints}),
		},
		{
			name:               "Delete keys by comment",
			request:            "DeleteKeys",
			expectedChanResult: "remove-keys-comment",
			params:             mustMarshal(jujuparams.ModifyUserSSHKeys{Keys: []string{"comment"}}),
		},
		{
			name:               "Delete keys by fingerprint",
			request:            "DeleteKeys",
			expectedChanResult: "remove-keys-fingerprint",
			params:             mustMarshal(jujuparams.ModifyUserSSHKeys{Keys: []string{"79:fc:60:93:ec:ce:42:fe:15:61:f2:fb:d6:22:43:6e"}}),
		},
		{
			name:        "Invalid method called",
			request:     "InvalidMethod",
			expectedErr: "unknown key manager request",
			params:      []byte{},
		},
	}

	for i, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			msg := rpcproxy.Message{RequestID: uint64(i + 1), Type: "KeyManager", Request: test.request, Params: test.params} // #nosec G115 accept integer conversion
			err := ws.WriteJSON(&msg)
			c.Assert(err, qt.IsNil)

			resp := rpcproxy.Message{}
			err = ws.ReadJSON(&resp)
			c.Assert(err, qt.IsNil)
			if test.expectedErr != "" {
				c.Assert(resp.Error, qt.Matches, test.expectedErr)
				return
			}

			c.Assert(resp.Error, qt.Equals, "")
			select {
			case res := <-sshFacadeChan:
				c.Assert(res, qt.Equals, test.expectedChanResult)
			case <-time.After(100 * time.Millisecond):
				c.Error("Expected SSH method was not called")
			}

		})
	}
	ws.Close()
	select {
	case <-time.After(5 * time.Second):
		c.Fatalf("model proxy did not return in time")
	case <-errChan: // Ensure go routines are cleaned up
	}
}

func mustMarshal(data any) []byte {
	out, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return out
}
