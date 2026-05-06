// Copyright 2025 Canonical.

package jimmhttp

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rpc"
)

// JujuManager provides the controller connection details used for model HTTP proxying.
type JujuManager interface {
	ControllerDetailsForModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
	ControllerDetailsForIncomingModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
}

// LoginTokenProvider mints a Juju login token for a user operating on a model and controller.
type LoginTokenProvider interface {
	NewLoginToken(ctx context.Context, modelTag names.ModelTag, controllerTag names.ControllerTag, user *openfga.User) ([]byte, error)
}

// HTTPProxyHandler is an handler that provides proxying capabilities.
// It uses the uuid in the path to proxy requests to model's controller.
type HTTPProxyHandler struct {
	Router             *chi.Mux
	authenicator       middleware.Authenticator
	jujuManager        JujuManager
	loginTokenProvider LoginTokenProvider
}

const (
	// all endpoints managed by this handler
	ProxyEndpoints = "/*"
)

// NewHTTPProxyHandler creates a proxy http handler.
func NewHTTPProxyHandler(authenticator middleware.Authenticator, jujuManager JujuManager, loginTokenProvider LoginTokenProvider) *HTTPProxyHandler {
	h := &HTTPProxyHandler{
		Router:             chi.NewRouter(),
		authenicator:       authenticator,
		jujuManager:        jujuManager,
		loginTokenProvider: loginTokenProvider,
	}
	h.SetupMiddleware()
	h.Router.HandleFunc(ProxyEndpoints, h.ProxyHTTP)
	return h
}

// Routes returns the grouped routers routes with group specific middlewares.
func (hph *HTTPProxyHandler) Routes() chi.Router {
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
		writeError(ctx, w, http.StatusBadRequest, errors.New(msg), msg)
		return
	}

	if !names.IsValidModel(modelUUID) {
		msg := "invalid model UUID format"
		writeError(ctx, w, http.StatusBadRequest, errors.New(msg), msg)
		return
	}

	user, err := middleware.IdentityFromContext(ctx)
	if err != nil {
		writeError(ctx, w, http.StatusUnauthorized, err, "failed to get authenticated user")
		return
	}

	controllerDetails, err := hph.jujuManager.ControllerDetailsForModel(ctx, modelUUID)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			writeError(ctx, w, http.StatusNotFound, err, "model not found")
			return
		}
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to get controller details")
		return
	}

	mt := names.NewModelTag(modelUUID)
	ct := names.NewControllerTag(controllerDetails.ControllerUUID)
	jwt, err := hph.loginTokenProvider.NewLoginToken(ctx, mt, ct, user)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to generate login token")
		return
	}

	requestHeaders := make(http.Header)
	requestHeaders.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(jwt))

	details := rpc.ConnectionDetails{
		Addresses:      controllerDetails.Addresses,
		PublicAddress:  controllerDetails.PublicAddress,
		CACertificate:  controllerDetails.CACertificate,
		TLSHostname:    controllerDetails.TLSHostname,
		RequestHeaders: requestHeaders,
	}

	rpc.ProxyHTTP(ctx, details, w, req)
}
