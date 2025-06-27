// Copyright 2025 Canonical.

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
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/streamproxy"
)

const (
	logTransferPath = "/migrate/logtransfer"
)

// streamControllerProxier serves any HTTP endpoints
// served against a controller's root address rather than
// a model specific path segment i.e. /model/{uuid}/*.
// Messages are handled by proxying them between the
// controller and client.
type streamControllerProxier struct {
	jimm *jimm.JIMM
}

// Authenticate implements WSServer.Authenticate
// It attempts to perform basic auth and will return an unauthorized error if auth fails.
func (s streamControllerProxier) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
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
//
// Currently the only endpoint we handle is /migrate/logtransfer for transferring
// logs after a model was successfully migrated.
// We expect the model UUID to be in the context, set by the http handler.
// If other handlers are added in the future, we may need to differentiate
// them and adjust the logic based on the path or some other criteria.
func (s streamControllerProxier) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
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

	user, err := s.jimm.LoginManager().UserLogin(ctx, auth.SessionIdentityFromContext(ctx))
	if err != nil {
		zapctx.Error(ctx, "user login error", zap.Error(err))
		writeError(err.Error(), errors.CodeUnauthorized)
		return
	}

	if !user.JimmAdmin {
		writeError("unauthorized", errors.CodeUnauthorized)
		return
	}

	// Note that by the time we reach log transfer, the model has been "activated"
	// meaning that we have removed the model from the IncomingModelMigration table as
	// migration was successful and we can now treat the UUID as one for a regular model.
	modelUUID := jimmhttp.MigratingModelUUIDFromContext(ctx)
	model, err := s.jimm.JujuManager().GetModel(ctx, modelUUID)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			zapctx.Error(ctx, "model not found", zap.String("modelUUID", modelUUID))
			writeError(fmt.Sprintf("model %q not found", modelUUID), errors.CodeModelNotFound)
			return
		}
		zapctx.Error(ctx, "failed to get model", zap.Error(err), zap.String("modelUUID", modelUUID))
		writeError(fmt.Sprintf("failed to get model: %s", err.Error()), errors.CodeServerError)
		return
	}

	api, err := s.jimm.Dialer.Dial(ctx, &model.Controller, names.ModelTag{}, nil, nil)
	if err != nil {
		zapctx.Error(ctx, "failed to dial controller", zap.Error(err))
		writeError(fmt.Sprintf("failed to dial controller: %s", err.Error()), errors.CodeConnectionFailed)
		return
	}
	defer api.Close()

	headers := http.Header{
		jujuparams.MigrationModelHTTPHeader: []string{modelUUID},
		jujuparams.JujuClientVersion:        []string{jimmhttp.ClientVersionFromContext(ctx)},
	}

	controllerStream, err := api.ConnectControllerStream(logTransferPath, nil, headers)
	if err != nil {
		zapctx.Error(ctx, "failed to connect stream", zap.Error(err))
		writeError(fmt.Sprintf("failed to connect stream: %s", err.Error()), errors.CodeConnectionFailed)
		return
	}

	streamproxy.ProxyStreams(ctx, clientConn, controllerStream)
}
