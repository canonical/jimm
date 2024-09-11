// Copyright 2024 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	jimmRPC "github.com/canonical/jimm/v3/internal/rpc"
)

// A streamProxier serves the the /log endpoint by proxying
// messages between the controller and client.
type streamProxier struct {
	// TODO(Kian): Refactor the apiServer to use the JIMM API rather than a concrete struct
	// then we can write unit tests for the stream proxier.
	apiServer
}

// Authenticate implements WSServer.Authenticate
// It attempts to perform basic auth and will return an unauthorized error if auth fails.
func (s streamProxier) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	_, password, ok := req.BasicAuth()
	if !ok {
		return ctx, errors.E(errors.CodeUnauthorized, "authentication missing")
	}
	jwtToken, err := s.jimm.OAuthAuthenticator.VerifySessionToken(password)
	if err != nil {
		return ctx, errors.E(errors.CodeUnauthorized, err)
	}
	email := jwtToken.Subject()
	ctx = auth.ContextWithSessionIdentity(ctx, email)
	return ctx, nil
}

// ServeWS implements jimmhttp.WSServer.
func (s streamProxier) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
	writeError := func(msg string, code errors.Code) {
		var errResult jujuparams.ErrorResult
		errResult.Error = &jujuparams.Error{
			Message: msg,
			Code:    string(code),
		}
		err := clientConn.WriteJSON(errResult)
		if err != nil {
			zapctx.Error(ctx, "failed to write error message to client", zap.Error(err), zap.Any("client message", errResult))
		}
	}

	user, err := s.jimm.UserLogin(ctx, auth.SessionIdentityFromContext(ctx))
	if err != nil {
		zapctx.Error(ctx, "user login error", zap.Error(err))
		writeError(err.Error(), errors.CodeUnauthorized)
		return
	}

	uuid, finalPath, err := modelInfoFromPath(jimmhttp.PathElementFromContext(ctx, "path"))
	if err != nil {
		zapctx.Error(ctx, "error parsing path", zap.Error(err))
		writeError(fmt.Sprintf("error parsing path: %s", err.Error()), errors.CodeBadRequest)
		return
	}

	modelTag := names.NewModelTag(uuid)

	if ok, err := checkModelAccessForUser(ctx, finalPath, user, modelTag); err != nil {
		writeError(err.Error(), errors.CodeUnauthorized)
		return
	} else if !ok {
		writeError(fmt.Sprintf("unauthorized access to endpoint: %s", finalPath), errors.CodeUnauthorized)
		return
	}

	model, err := s.jimm.GetModel(ctx, uuid)
	if err != nil {
		writeError(err.Error(), errors.CodeModelNotFound)
		return
	}

	api, err := s.jimm.Dialer.Dial(ctx, &model.Controller, model.ResourceTag(), nil)
	if err != nil {
		zapctx.Error(ctx, "failed to dial controller", zap.Error(err))
		writeError(fmt.Sprintf("failed to dial controller: %s", err.Error()), errors.CodeConnectionFailed)
		return
	}
	defer api.Close()

	controllerStream, err := api.ConnectStream(finalPath, nil)
	if err != nil {
		zapctx.Error(ctx, "failed to connect stream", zap.Error(err))
		writeError(fmt.Sprintf("failed to connect stream: %s", err.Error()), errors.CodeConnectionFailed)
		return
	}

	jimmRPC.ProxyStreams(ctx, clientConn, controllerStream)
}
