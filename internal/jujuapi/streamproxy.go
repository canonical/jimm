// Copyright 2024 Canonical.
package jujuapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// A streamProxier serves all HTTP endpoints by proxying
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
	model, err := s.getModel(ctx, uuid)
	if err != nil {
		writeError(err.Error(), errors.CodeModelNotFound)
		return
	}
	if ok, err := checkPermission(ctx, finalPath, user, model.ResourceTag()); err != nil {
		writeError(err.Error(), errors.CodeUnauthorized)
		return
	} else if !ok {
		writeError(fmt.Sprintf("unauthorized access to endpoint: %s", finalPath), errors.CodeUnauthorized)
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
	proxyStreams(ctx, clientConn, controllerStream)
}

func (s streamProxier) getModel(ctx context.Context, modelUUID string) (dbmodel.Model, error) {
	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUID,
			Valid:  modelUUID != "",
		},
	}
	if err := s.jimm.Database.GetModel(context.Background(), &model); err != nil {
		zapctx.Error(ctx, "failed to find model", zap.String("uuid", modelUUID), zap.Error(err))
		return dbmodel.Model{}, fmt.Errorf("failed to find model: %s", err.Error())
	}
	return model, nil
}

// proxyStreams starts a simple proxy for 2 websockets.
// After starting the proxy we listen for the first error
// returned and then close both connections before waiting
// for an error from the second connection.
func proxyStreams(ctx context.Context, src, dst base.Stream) {
	errChan := make(chan error, 2)
	go func() { errChan <- proxy(src, dst) }()
	go func() { errChan <- proxy(dst, src) }()
	err := <-errChan
	if err != nil {
		zapctx.Error(ctx, "error from stream proxy", zap.Error(err))
	}
	dst.Close()
	src.Close()
	err = <-errChan
	if err != nil {
		zapctx.Error(ctx, "error from stream proxy", zap.Error(err))
	}
}

func proxy(src base.Stream, dst base.Stream) error {
	for {
		var data map[string]any
		err := src.ReadJSON(&data)
		if err != nil {
			if unexpectedReadError(err) {
				return err
			}
			return nil
		}
		err = dst.WriteJSON(data)
		if err != nil {
			return err
		}
	}
}

func unexpectedReadError(err error) bool {
	return false
}

func checkPermission(ctx context.Context, path string, u *openfga.User, mt names.ModelTag) (bool, error) {
	switch path {
	case "log":
		return u.IsModelReader(ctx, mt)
	default:
		return false, errors.E("unknown endpoint " + path)
	}
}
