// Copyright 2016 Canonical Ltd.

// Package jujuapi implements API endpoints for the juju API.
package jujuapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/httprequest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/ctxutil"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	return append(
		jemerror.Mapper.Handlers(func(p httprequest.Params) (*handler, error) {
			ctx := ctxutil.Join(ctx, p.Context)
			ctx = zapctx.WithFields(ctx, zap.String("req-id", httprequest.RequestUUID(ctx)))
			return &handler{
				context: ctx,
				params:  params.Params,
				jem:     params.JEMPool.JEM(ctx),
			}, nil
		}),
		newWebSocketHandler(ctx, params),
		newRootWebSocketHandler(ctx, params, "/"),
		newRootWebSocketHandler(ctx, params, "/api"),
	), nil
}

func newWebSocketHandler(ctx context.Context, params jemserver.HandlerParams) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   "/model/:modeluuid/api",
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			ctx := ctxutil.Join(r.Context(), ctx)
			servermon.ConcurrentWebsocketConnections.Inc()
			defer servermon.ConcurrentWebsocketConnections.Dec()
			j := params.JEMPool.JEM(ctx)
			defer j.Close()
			wsServer := newWSServer(ctx, j, params.AuthenticatorPool, params.Params, p.ByName("modeluuid"))
			wsServer.ServeHTTP(w, r)
		},
	}
}

func newRootWebSocketHandler(ctx context.Context, params jemserver.HandlerParams, path string) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   path,
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			// TODO add unique id to context or derive it from http request.
			ctx := zapctx.WithFields(ctx, zap.Bool("websocket", true))
			j := params.JEMPool.JEM(ctx)
			defer j.Close()
			wsServer := newWSServer(ctx, j, params.AuthenticatorPool, params.Params, "")
			wsServer.ServeHTTP(w, r)
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
	ctx := ctxutil.Join(h.context, p.Context)
	if h.params.GUILocation == "" {
		return errgo.WithCausef(nil, params.ErrNotFound, "no GUI location specified")
	}
	m, err := h.jem.DB.ModelFromUUID(ctx, arg.UUID)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	http.Redirect(p.Response, p.Request, fmt.Sprintf("%s/u/%s/%s", h.params.GUILocation, m.Path.User, m.Path.Name), http.StatusMovedPermanently)
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
