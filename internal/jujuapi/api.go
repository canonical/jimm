// Copyright 2016 Canonical Ltd.

// Package jujuapi implements API endpoints for the juju API.
package jujuapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"

	"github.com/canonical/jimm/internal/jem"
	"github.com/canonical/jimm/internal/jemerror"
	"github.com/canonical/jimm/internal/jemserver"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/internal/servermon"
	"github.com/canonical/jimm/internal/zapctx"
	"github.com/canonical/jimm/internal/zaputil"
	"github.com/canonical/jimm/params"
)

func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	srv := &httprequest.Server{
		ErrorMapper: jemerror.Mapper,
	}

	return append(
		srv.Handlers(func(p httprequest.Params) (*handler, context.Context, error) {
			return &handler{
				context: p.Context,
				params:  params.Params,
				jem:     params.JEMPool.JEM(ctx),
			}, ctx, nil
		}),
		newWebSocketHandler(ctx, params),
		newRootWebSocketHandler(ctx, params, "/api"),
	), nil
}

func newWebSocketHandler(ctx context.Context, params jemserver.HandlerParams) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   "/model/:UUID/api",
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			ctx := zapctx.WithFields(r.Context(), zap.Bool("websocket", true))
			servermon.ConcurrentWebsocketConnections.Inc()
			defer servermon.ConcurrentWebsocketConnections.Dec()
			j := params.JEMPool.JEM(ctx)
			defer j.Close()
			wsServer := newWSServer(j, params.Authenticator, params.Params, p.ByName("UUID"))
			wsServer.ServeHTTP(w, r.WithContext(ctx))
		},
	}
}

func newRootWebSocketHandler(ctx context.Context, params jemserver.HandlerParams, path string) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   path,
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			// _TODO add unique id to context or derive it from http request.
			ctx := zapctx.WithFields(r.Context(), zap.Bool("websocket", true))
			j := params.JEMPool.JEM(ctx)
			defer j.Close()
			wsServer := newWSServer(j, params.Authenticator, params.Params, "")
			wsServer.ServeHTTP(w, r.WithContext(ctx))
		},
	}
}

type handler struct {
	context context.Context
	params  jemserver.Params
	jem     *jem.JEM
}

func (h *handler) Close() error {
	h.jem.Close()
	return nil
}

type guiRequest struct {
	httprequest.Route `httprequest:"GET /gui/:UUID"`
	UUID              string `httprequest:",path"`
}

// GUI provides a GUI by redirecting to the store front.
func (h *handler) GUI(p httprequest.Params, arg *guiRequest) error {
	ctx := p.Context
	if h.params.GUILocation == "" {
		return errgo.WithCausef(nil, params.ErrNotFound, "no GUI location specified")
	}
	m := mongodoc.Model{UUID: arg.UUID}
	if err := h.jem.DB.GetModel(ctx, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	http.Redirect(p.Response, p.Request, fmt.Sprintf("%s/u/%s/%s", h.params.GUILocation, m.Path.User, m.Path.Name), http.StatusMovedPermanently)
	return nil
}

type modelCommandsRequest struct {
	httprequest.Route `httprequest:"GET /model/:UUID/commands"`
	UUID              string `httprequest:",path"`
}

// ModelCommands redirects the request to the controller responsible for the model.
func (h *handler) ModelCommands(p httprequest.Params, arg *modelCommandsRequest) error {
	ctx := p.Context
	m := mongodoc.Model{UUID: arg.UUID}
	if err := h.jem.DB.GetModel(ctx, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	controller := mongodoc.Controller{Path: m.Controller}
	if err := h.jem.DB.GetController(ctx, &controller); err != nil {
		return errgo.Mask(err)
	}
	addrs := mongodoc.Addresses(controller.HostPorts)
	if len(addrs) == 0 {
		return errgo.New("expected at least 1 address, got 0")
	}
	conn, err := websocketUpgrader.Upgrade(p.Response, p.Request, nil)
	if err != nil {
		zapctx.Error(ctx, "cannot upgrade websocket", zaputil.Error(err))
		return errgo.Mask(err)
	}
	codec := jsoncodec.NewWebsocketConn(conn)
	defer codec.Close()
	err = codec.Send(struct {
		RedirectTo string `json:"redirect-to"`
	}{
		RedirectTo: fmt.Sprintf("wss://%s/model/%s/commands", addrs[0], arg.UUID),
	})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}
