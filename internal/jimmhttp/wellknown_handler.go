// Copyright 2025 Canonical.

package jimmhttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
)

const (
	JwksEndpoint = "/jwks.json"
)

// JWKSProvider defines the interface for retrieving JWKS material and cache max age.
type JWKSProvider interface {
	Get(ctx context.Context) (jwk.Set, error)
}

// WellKnownHandler holds the grouped router to be mounted and
// any service checks we wish to register.
// Implements jimmhttp.JIMMHttpHandler
type WellKnownHandler struct {
	Router       *chi.Mux
	JWKSProvider JWKSProvider
}

// NewWellKnownHandler returns a new WellKnownHandler
func NewWellKnownHandler(provider JWKSProvider) *WellKnownHandler {
	return &WellKnownHandler{Router: chi.NewRouter(), JWKSProvider: provider}
}

// Routes returns the grouped routers routes with group specific middlewares.
func (wkh *WellKnownHandler) Routes() chi.Router {
	wkh.SetupMiddleware()
	wkh.Router.Get(JwksEndpoint, wkh.JWKS)
	return wkh.Router
}

// SetupMiddleware applies middlewares.
func (wkh *WellKnownHandler) SetupMiddleware() {
	wkh.Router.Use(
		render.SetContentType(
			render.ContentTypeJSON,
		),
	)
}

// JWKS handles /jwks.json, this represents a mimic of your ordinary IdP JWKS endpoint.
// The purpose of this is to allow juju controllers to retrieve the public key from JIMM
// and decode the presented JWT.
func (wkh *WellKnownHandler) JWKS(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	if wkh == nil || wkh.JWKSProvider == nil {
		zapctx.Error(ctx, "nil reference in JWKS handler")
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, errors.Error{Code: errors.CodeJWKSRetrievalFailed, Message: "JWKS does not exist"})
		return
	}
	ks, err := wkh.JWKSProvider.Get(ctx)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		zapctx.Error(ctx, "HTTP error", zap.NamedError("/jwks.json", fmt.Errorf("failed to retrieve JWKS: %w", err)))
		render.JSON(w, r, errors.Error{Code: errors.CodeJWKSRetrievalFailed, Message: "failed to retrieve JWKS"})
		return
	}

	// Cache-control of 10 minutes
	w.Header().Set("Cache-Control", "max-age=600")
	render.JSON(w, r, ks)
}
