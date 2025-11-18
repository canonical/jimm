// Copyright 2025 Canonical.

package jimmhttp

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
)

const (
	JwksEndpoint = "/jwks.json"
)

// WellKnownHandler holds the grouped router to be mounted and
// any service checks we wish to register.
// Implements jimmhttp.JIMMHttpHandler
type WellKnownHandler struct {
	Router          *chi.Mux
	CredentialStore credentials.CredentialStore
}

// NewWellKnownHandler returns a new WellKnownHandler
func NewWellKnownHandler(cs credentials.CredentialStore) *WellKnownHandler {
	return &WellKnownHandler{Router: chi.NewRouter(), CredentialStore: cs}
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
//
// The JWKS is expected to be cached by the client, where the expiry time
// is the expiry time persisted for this set in our credential store.
func (wkh *WellKnownHandler) JWKS(w http.ResponseWriter, r *http.Request) {
	const op = errors.Op("wellknownapi.JWKS")
	ctx := r.Context()
	if wkh == nil || wkh.CredentialStore == nil {
		zapctx.Error(ctx, "nil reference in JWKS handler")
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, errors.E(op, errors.CodeJWKSRetrievalFailed, "JWKS does not exist"))
		return
	}
	ks, err := wkh.CredentialStore.GetJWKS(ctx)

	if err != nil && errors.ErrorCode(err) == errors.CodeNotFound {
		w.WriteHeader(http.StatusNotFound)
		zapctx.Error(ctx, "HTTP error", zap.NamedError("/jwks.json", errors.E(op, errors.CodeJWKSRetrievalFailed, "JWKS does not exist yet", err)))
		render.JSON(w, r, errors.E(op, errors.CodeNotFound, "JWKS does not exist yet"))
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		zapctx.Error(ctx, "HTTP error", zap.NamedError("/jwks.json", errors.E(op, errors.CodeJWKSRetrievalFailed, "failed to retrieve JWKS", err)))
		render.JSON(w, r, errors.E(op, errors.CodeJWKSRetrievalFailed, "failed to retrieve JWKS"))
		return
	}

	expiry, err := wkh.CredentialStore.GetJWKSExpiry(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		zapctx.Error(ctx, "HTTP error", zap.NamedError("/jwks.json", errors.E(op, errors.CodeJWKSRetrievalFailed, "failed to retrieve JWKS expiry", err)))
		render.JSON(w, r, errors.E(op, errors.CodeJWKSRetrievalFailed, "something went wrong..."))
		return
	}

	// Set cache headers to a maximum age of 10 minutes (600 seconds)
	// to ensure clients do not cache for too long.
	// If expiry is less than 10 minutes, use the actual expiry
	const maxAgeSeconds = 600
	now := time.Now().UTC()
	actualMaxAge := int64(maxAgeSeconds)

	// If the expiry is sooner than now + maxAgeSeconds, use the expiry instead.
	// If the expiry is in the past, set a low cache value of 30s.
	if expiry.After(now) {
		remaining := int64(expiry.Sub(now).Seconds())
		if remaining < maxAgeSeconds {
			actualMaxAge = remaining
		}
	} else {
		actualMaxAge = 10
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("must-revalidate, max-age=%d", actualMaxAge))
	render.JSON(w, r, ks)
}
