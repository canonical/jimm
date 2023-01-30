package wellknownapi

import (
	"net/http"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm/credentials"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
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
// and decode the presented forwarded JWT.
func (wkh *WellKnownHandler) JWKS(w http.ResponseWriter, r *http.Request) {
	const op = errors.Op("wellknownapi.JWKS")
	ctx := r.Context()
	ks, err := wkh.CredentialStore.GetJWKS(ctx)
	if err != nil {
		render.JSON(w, r, errors.E(op, "jwks retrieval failure", "failed to retrieve JWKS", err))
	}
	render.JSON(w, r, ks)
}
