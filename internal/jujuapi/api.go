// Copyright 2016 Canonical Ltd.

// Package jujuapi implements API endpoints for the juju API.
package jujuapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"gopkg.in/httprequest.v1"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

func NewAPIHandler(ctx context.Context, jimm *jimm.JIMM, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	srv := &httprequest.Server{
		ErrorMapper: errorMapper,
	}

	return append(
		srv.Handlers(func(p httprequest.Params) (*handler, context.Context, error) {
			return &handler{
				context: p.Context,
				params:  params.Params,
				jimm:    jimm,
			}, ctx, nil
		}),
		newWebSocketHandler(ctx, jimm, params),
		newRootWebSocketHandler(ctx, jimm, params, "/"),
		newRootWebSocketHandler(ctx, jimm, params, "/api"),
	), nil
}

func newWebSocketHandler(ctx context.Context, jimm *jimm.JIMM, params jemserver.HandlerParams) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   "/model/:UUID/api",
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			ctx := zapctx.WithFields(r.Context(), zap.Bool("websocket", true))
			servermon.ConcurrentWebsocketConnections.Inc()
			defer servermon.ConcurrentWebsocketConnections.Dec()
			wsServer := newWSServer(jimm, params.Params, p.ByName("UUID"))
			wsServer.ServeHTTP(w, r.WithContext(ctx))
		},
	}
}

func newRootWebSocketHandler(ctx context.Context, jimm *jimm.JIMM, params jemserver.HandlerParams, path string) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   path,
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			// _TODO add unique id to context or derive it from http request.
			ctx := zapctx.WithFields(r.Context(), zap.Bool("websocket", true))
			wsServer := newWSServer(jimm, params.Params, "")
			wsServer.ServeHTTP(w, r.WithContext(ctx))
		},
	}
}

type handler struct {
	context context.Context
	params  jemserver.Params
	jimm    *jimm.JIMM
}

type guiRequest struct {
	httprequest.Route `httprequest:"GET /gui/:UUID"`
	UUID              string `httprequest:",path"`
}

// GUI provides a GUI by redirecting to the store front.
func (h *handler) GUI(p httprequest.Params, arg *guiRequest) error {
	ctx := p.Context
	if h.params.GUILocation == "" {
		return errors.E(errors.CodeNotFound, "no GUI location specified")
	}
	m := dbmodel.Model{
		UUID: sql.NullString{
			String: arg.UUID,
			Valid:  true,
		},
	}
	if err := h.jimm.Database.GetModel(ctx, &m); err != nil {
		return err
	}
	user := strings.TrimSuffix(m.OwnerUsername, "@external")
	http.Redirect(p.Response, p.Request, fmt.Sprintf("%s/u/%s/%s", h.params.GUILocation, user, m.Name), http.StatusMovedPermanently)
	return nil
}

type guiArchiveRequest struct {
	httprequest.Route `httprequest:"GET /gui-archive"`
}

// GUIArchive provides information on GUI versions for compatibility with Juju
// controllers. In this case, no versions are returned.
func (h *handler) GUIArchive(*guiArchiveRequest) (jujuparams.GUIArchiveResponse, error) {
	return jujuparams.GUIArchiveResponse{}, nil
}

type modelCommandsRequest struct {
	httprequest.Route `httprequest:"GET /model/:UUID/commands"`
	UUID              string `httprequest:",path"`
}

// ModelCommands redirects the request to the controller responsible for the model.
func (h *handler) ModelCommands(p httprequest.Params, arg *modelCommandsRequest) error {
	ctx := p.Context
	m := dbmodel.Model{
		UUID: sql.NullString{
			String: arg.UUID,
			Valid:  true,
		},
	}
	if err := h.jimm.Database.GetModel(ctx, &m); err != nil {
		return err
	}
	conn, err := websocketUpgrader.Upgrade(p.Response, p.Request, nil)
	if err != nil {
		zapctx.Error(ctx, "cannot upgrade websocket", zaputil.Error(err))
		return err
	}
	codec := jsoncodec.NewWebsocketConn(conn)
	defer codec.Close()
	addr := m.Controller.PublicAddress
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", m.Controller.Addresses[0][0].Value, m.Controller.Addresses[0][0].Port)
	}
	err = codec.Send(struct {
		RedirectTo string `json:"redirect-to"`
	}{
		RedirectTo: fmt.Sprintf("wss://%s/model/%s/commands", addr, arg.UUID),
	})
	if err != nil {
		return err
	}
	return nil
}

func errorMapper(ctx context.Context, err error) (status int, body interface{}) {
	body = &params.Error{
		Message: err.Error(),
		Code:    params.ErrorCode(errors.ErrorCode(err)),
	}
	switch errors.ErrorCode(err) {
	case errors.CodeBadRequest:
		status = http.StatusBadRequest
	case errors.CodeNotFound:
		status = http.StatusNotFound
	default:
		status = http.StatusInternalServerError
	}
	return status, body
}
