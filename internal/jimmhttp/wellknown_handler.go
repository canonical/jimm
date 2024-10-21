// Copyright 2024 Canonical.
package jimmhttp

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
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
	wkh.Router.Get("/jwks.json", wkh.JWKS)
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

	// Calculate remaining max-age from now to expiry time
	maxAge := expiry.Sub(time.Now().UTC())
	// Format expiry into expires header valid string (RFC 1123)
	expires := expiry.Format(time.RFC1123)

	// The cache is shared and set to:
	//	- must-revalidate (to indicate it is a long running cache)
	//	- maximum age (which is likely months, in our case 3)
	// 	- immutable (so the client of this JWKS knows it will not change until the rotation date)

	w.Header().Add("Cache-Control", fmt.Sprintf("must-revalidate, max-age=%d, immutable", int64(math.Floor(maxAge.Seconds()))))
	// We also use expires as I've noticed some JWK cache clients in golang specifically
	// look at the expires header over max-age directive... No idea why.
	w.Header().Add("Expires", expires)
	render.JSON(w, r, ks)
}
