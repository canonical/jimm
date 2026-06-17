// Copyright 2025 Canonical.

package rpcproxy

import (
	"context"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/utils"
)

type proxySession struct {
	clientConn WebsocketConnection
	proxy      *clientProxy
	errChan    chan error
}

func newProxySession(helpers ProxyHelpers) (*proxySession, error) {
	if err := helpers.validate(); err != nil {
		return nil, err
	}

	errChan := make(chan error, 2)
	tracker := newInflightTracker()
	client := &writeLockConn{conn: helpers.ConnClient}
	proxy := &clientProxy{
		modelProxy: modelProxy{
			src:                     client,
			inflight:                tracker,
			tokenGen:                helpers.TokenGen,
			auditLog:                helpers.AuditLog,
			conversationId:          utils.NewConversationID(),
			loginService:            helpers.LoginService,
			authenticatedIdentityID: helpers.AuthenticatedIdentityID,
			redirectInfo:            helpers.RedirectInfo,
		},
		errChan:              errChan,
		createControllerConn: helpers.ConnectController,
	}

	return &proxySession{
		clientConn: helpers.ConnClient,
		proxy:      proxy,
		errChan:    errChan,
	}, nil
}

func (session *proxySession) run(ctx context.Context) error {
	session.proxy.wg.Go(func() {
		session.errChan <- session.proxy.start(ctx)
	})

	err := session.wait(ctx)
	// Close the client connection to ensure everything is cleaned up.
	// Normally the client would do this but we also do it here in case the
	// connection to the controller fails and we want to trigger cleanup.
	session.clientConn.Close()
	session.proxy.wg.Wait()
	return err
}

func (session *proxySession) wait(ctx context.Context) error {
	select {
	case err := <-session.errChan:
		if err != nil {
			zapctx.Debug(ctx, "Proxy error", zap.Error(err))
		}
		return err
	case <-ctx.Done():
		zapctx.Debug(ctx, "Context cancelled")
		return errors.New("Context cancelled")
	}
}

func (helpers ProxyHelpers) validate() error {
	if helpers.ConnectController == nil {
		return errors.New("missing controller connect function")
	}
	if helpers.AuditLog == nil {
		return errors.New("missing audit log function")
	}
	if helpers.LoginService == nil {
		return errors.New("missing login service function")
	}
	if helpers.RedirectInfo == nil {
		return errors.New("missing redirect info function")
	}
	return nil
}
