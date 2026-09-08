// Copyright 2025 Canonical.

package jimmhttp

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
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

// LoginTokenProvider mints a Juju login token for a user operating on a controller.
type LoginTokenProvider interface {
	NewCallerLoginToken(ctx context.Context, resourceTags []names.Tag, ctl *dbmodel.Controller, user *openfga.User) ([]byte, error)
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

	requestHeaders, err := callerAuthorizationHeader(ctx, hph.loginTokenProvider, controllerDetails, modelUUID, user)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to generate login token")
		return
	}

	details := rpc.ConnectionDetails{
		Addresses:      controllerDetails.Addresses,
		PublicAddress:  controllerDetails.PublicAddress,
		CACertificate:  controllerDetails.CACertificate,
		TLSHostname:    controllerDetails.TLSHostname,
		RequestHeaders: requestHeaders,
	}

	rpc.ProxyHTTP(ctx, details, w, req)
}

// callerAuthorizationHeader mints a login token scoped to the caller's
// real permissions and returns an Authorization header carrying it. For
// controllers below the fix boundary (Juju <=3.6.23), a superuser token
// is used as a fallback due to a Juju bug where model-admin JWT claims
// are not honoured.
func callerAuthorizationHeader(ctx context.Context, tokenProvider LoginTokenProvider, controllerDetails juju.ControllerConnectionDetails, modelUUID string, user *openfga.User) (http.Header, error) {
	mt := names.NewModelTag(modelUUID)
	ctl := &dbmodel.Controller{UUID: controllerDetails.ControllerUUID, AgentVersion: controllerDetails.AgentVersion}
	jwt, err := tokenProvider.NewCallerLoginToken(ctx, []names.Tag{mt}, ctl, user)
	if err != nil {
		return nil, err
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(jwt))
	return header, nil
}
