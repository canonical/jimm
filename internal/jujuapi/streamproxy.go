package jujuapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/gorilla/websocket"
	"github.com/juju/juju/api/base"
	jujuParams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// A streamProxier serves all HTTP endpoints by proxying
// messages between the controller and client.
type streamProxier struct {
	apiServer
}

func (s streamProxier) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	_, password, ok := req.BasicAuth()
	if !ok {
		return ctx, nil
	}
	jwtToken, err := s.jimm.OAuthAuthenticator.VerifySessionToken(password)
	if err != nil {
		return ctx, err
	}
	email := jwtToken.Subject()
	ctx = auth.ContextWithSessionIdentity(ctx, email)
	return ctx, nil
}

// ServeWS implements jimmhttp.WSServer.
func (s streamProxier) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
	identity := auth.SessionIdentityFromContext(ctx)
	writeError := func(msg string, code errors.Code) {
		var errResult jujuParams.ErrorResult
		errResult.Error = &jujuParams.Error{
			Message: msg,
			Code:    string(code),
		}
		clientConn.WriteJSON(errResult)
	}
	if identity == "" {
		writeError("identity not found", errors.CodeUnauthorized)
		return
	}
	uuid, finalPath, err := modelInfoFromPath(jimmhttp.PathElementFromContext(ctx, "path"))
	if err != nil {
		zapctx.Error(ctx, "error parsing path", zap.Error(err))
		writeError(fmt.Sprintf("error parsing path: %s", err.Error()), errors.CodeBadRequest)
		return
	}
	i, err := dbmodel.NewIdentity(identity)
	if err != nil {
		writeError(err.Error(), errors.CodeNotFound)
		return
	}
	user := openfga.NewUser(i, s.jimm.OpenFGAClient)
	model := dbmodel.Model{
		UUID: sql.NullString{
			String: uuid,
			Valid:  uuid != "",
		},
	}
	if err := s.jimm.Database.GetModel(context.Background(), &model); err != nil {
		zapctx.Error(ctx, "failed to find model", zap.String("uuid", uuid), zap.Error(err))
		writeError(fmt.Sprintf("failed to find model: %s", err.Error()), errors.CodeModelNotFound)
		return
	}
	mt := model.ResourceTag()
	if ok, err := checkPermission(ctx, finalPath, user, mt); err != nil {
		writeError(err.Error(), errors.CodeUnauthorized)
		return
	} else if !ok {
		writeError("unauthorized", errors.CodeUnauthorized)
		return
	}
	api, err := s.jimm.Dialer.Dial(ctx, &model.Controller, mt, nil)
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
	errChan := make(chan error, 2)
	go func() { errChan <- proxy(controllerStream, clientConn) }()
	go func() { errChan <- proxy(clientConn, controllerStream) }()
	<-errChan
	controllerStream.Close()
	clientConn.Close()
	<-errChan
}

func proxy(in base.Stream, out base.Stream) error {
	for {
		var data map[string]any
		err := in.ReadJSON(&data)
		if err != nil {
			return err
		}
		err = out.WriteJSON(data)
		if err != nil {
			return err
		}
	}
}

func checkPermission(ctx context.Context, path string, u *openfga.User, mt names.ModelTag) (bool, error) {
	switch path {
	case "log":
		return u.IsModelReader(ctx, mt)
	default:
		return false, errors.E("unknown endpoint " + path)
	}
}
