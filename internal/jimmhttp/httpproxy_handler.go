// Copyright 2024 Canonical.

package jimmhttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/middleware"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rpc"
)

type HTTPProxyHandler struct {
	Router *chi.Mux
	jimm   *jimm.JIMM
}

const (
	ProxyEndpoints = "/*"
)

// NewHTTPProxyHandler creates a proxy hhtp handler.
func NewHTTPProxyHandler(jimm *jimm.JIMM) *HTTPProxyHandler {
	return &HTTPProxyHandler{Router: chi.NewRouter(), jimm: jimm}
}

// Routes returns the grouped routers routes with group specific middlewares.
func (hph *HTTPProxyHandler) Routes() chi.Router {
	hph.SetupMiddleware()
	hph.Router.HandleFunc(ProxyEndpoints, hph.ProxyHTTP)
	return hph.Router
}

func (hph *HTTPProxyHandler) RegisterEndpoints(mux *chi.Mux) {
	hph.SetupMiddleware()
	mux.HandleFunc(ProxyEndpoints, hph.ProxyHTTP)
}

// SetupMiddleware applies authn and authz middlewares.
func (hph *HTTPProxyHandler) SetupMiddleware() {
	hph.Router.Use(func(h http.Handler) http.Handler {
		return middleware.AuthenticateWithSessionTokenViaBasicAuth(h, hph.jimm)
	})
	hph.Router.Use(func(h http.Handler) http.Handler {
		return middleware.AuthorizeUserForModelAccess(h, ofganames.WriterRelation)
	})
}

// ProxyHTTP extracts the model uuid from the path to proxy the request to the right controller.
func (hph *HTTPProxyHandler) ProxyHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	writeError := func(msg string, code int) {
		w.WriteHeader(code)
		_, err := w.Write([]byte(msg))
		if err != nil {
			zapctx.Error(ctx, "cannot write to connection", zap.Error(err))
		}
	}
	modelUUID := chi.URLParam(req, "uuid")
	if modelUUID == "" {
		writeError("cannot parse path", http.StatusUnprocessableEntity)
		return
	}
	model, err := hph.jimm.GetModel(ctx, modelUUID)
	if err != nil {
		writeError("cannot get model", http.StatusNotFound)
		return
	}
	u, p, err := hph.jimm.GetCredentialStore().GetControllerCredentials(ctx, model.Controller.Name)
	if err != nil {
		writeError("cannot retrieve credentials", http.StatusNotFound)
		return
	}
	req.SetBasicAuth(names.NewUserTag(u).String(), p)

	err = rpc.ProxyHTTP(ctx, &model.Controller, w, req)
	if err != nil {
		http.Error(w, "Gateway timeout", http.StatusGatewayTimeout)
	}
}
