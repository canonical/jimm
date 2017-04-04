// Copyright 2016 Canonical Ltd.

// Package jujuapi implements API endpoints for the juju API.
package jujuapi

import (
	"fmt"
	"net/http"

	"github.com/juju/httprequest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/julienschmidt/httprouter"
	"github.com/uber-go/zap"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/ctxutil"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/params"
)

func NewAPIHandler(ctx context.Context, jp *jem.Pool, ap *auth.Pool, params jemserver.Params) ([]httprequest.Handler, error) {
	return append(
		jemerror.Mapper.Handlers(func(p httprequest.Params) (*handler, error) {
			ctx := ctxutil.Join(ctx, p.Context)
			ctx = zapctx.WithFields(ctx, zap.String("req-id", httprequest.RequestUUID(ctx)))
			return &handler{
				context: ctx,
				params:  params,
				jem:     jp.JEM(ctx),
			}, nil
		}),
		newWebSocketHandler(ctx, jp, ap, params),
		newRootWebSocketHandler(ctx, jp, ap, params, "/"),
		newRootWebSocketHandler(ctx, jp, ap, params, "/api"),
	), nil
}

func newWebSocketHandler(ctx context.Context, jp *jem.Pool, ap *auth.Pool, params jemserver.Params) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   "/model/:modeluuid/api",
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			servermon.ConcurrentWebsocketConnections.Inc()
			defer servermon.ConcurrentWebsocketConnections.Dec()
			j := jp.JEM(ctx)
			defer j.Close()
			wsServer := newWSServer(ctx, j, ap, params, p.ByName("modeluuid"))
			wsServer.ServeHTTP(w, r)
		},
	}
}

func newRootWebSocketHandler(ctx context.Context, jp *jem.Pool, ap *auth.Pool, params jemserver.Params, path string) httprequest.Handler {
	return httprequest.Handler{
		Method: "GET",
		Path:   path,
		Handle: func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			// TODO add unique id to context or derive it from http request.
			ctx := zapctx.WithFields(ctx, zap.Bool("websocket", true))
			j := jp.JEM(ctx)
			defer j.Close()
			wsServer := newWSServer(ctx, j, ap, params, "")
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
