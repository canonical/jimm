// Copyright 2025 Canonical.

package jimmhttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/middleware"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rpc"
)

type CredentialStore interface {
	ControllerDetailsForModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
	ControllerDetailsForIncomingModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
}

// HTTPProxyHandler is an handler that provides proxying capabilities.
// It uses the uuid in the path to proxy requests to model's controller.
type HTTPProxyHandler struct {
	Router          *chi.Mux
	authenicator    middleware.Authenticator
	credentialStore CredentialStore
}

const (
	// all endpoints managed by this handler
	ProxyEndpoints = "/*"
)

// NewHTTPProxyHandler creates a proxy http handler.
func NewHTTPProxyHandler(authenticator middleware.Authenticator, credentialStore CredentialStore) *HTTPProxyHandler {
	return &HTTPProxyHandler{
		Router:          chi.NewRouter(),
		authenicator:    authenticator,
		credentialStore: credentialStore,
	}
}

// Routes returns the grouped routers routes with group specific middlewares.
func (hph *HTTPProxyHandler) Routes() chi.Router {
	hph.SetupMiddleware()
	hph.Router.HandleFunc(ProxyEndpoints, hph.ProxyHTTP)
	return hph.Router
}

// SetupMiddleware applies authn and authz middlewares.
func (hph *HTTPProxyHandler) SetupMiddleware() {
	hph.Router.Use(func(h http.Handler) http.Handler {
		return middleware.AuthenticateViaBasicAuth(h, hph.authenicator)
	})
	hph.Router.Use(func(h http.Handler) http.Handler {
		return middleware.AuthorizeUserForModelAccess(h, ofganames.WriterRelation)
	})
}

// ProxyHTTP extracts the model uuid from the path to proxy the request to the right controller.
func (hph *HTTPProxyHandler) ProxyHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	modelUUID := chi.URLParam(req, "uuid")
	if modelUUID == "" {
		msg := "cannot parse model UUID from path"
		writeError(ctx, w, http.StatusBadRequest, errors.E(msg), msg)
		return
	}

	if !names.IsValidModel(modelUUID) {
		msg := "invalid model UUID format"
		writeError(ctx, w, http.StatusBadRequest, errors.E(msg), msg)
		return
	}

	controllerDetails, err := hph.credentialStore.ControllerDetailsForModel(ctx, modelUUID)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			writeError(ctx, w, http.StatusNotFound, err, "model not found")
			return
		}
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to get controller details")
		return
	}

	details := rpc.ConnectionDetails{
		Addresses:     controllerDetails.Addresses,
		PublicAddress: controllerDetails.PublicAddress,
		CACertificate: controllerDetails.CACertificate,
		TLSHostname:   controllerDetails.TLSHostname,
		Username:      controllerDetails.Credentials.AdminIdentityName,
		Password:      controllerDetails.Credentials.AdminPassword,
	}

	rpc.ProxyHTTP(ctx, details, w, req)
}
