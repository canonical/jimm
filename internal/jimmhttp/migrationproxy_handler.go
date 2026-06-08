// Copyright 2025 Canonical.

package jimmhttp

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/rpc"
)

// MigrationProxyHandler is an handler that provides proxying,
// specifically for HTTP-based model migration endpoints. These
// are used primarily to send data like charms/resources/tools
// to the target controller during a model migration.
// It differs slighly from the regular HTTPProxyHandler in that it
// 1. Uses a custom HTTP header to pass the model UUID instead of extracting it
// from the URL path.
// 2. Performs different checks to ensure the model is in a
// migration state before proxying the request to the controller.
// 3. Requires the user to be a JIMM admin, rather than just a model writer.
type MigrationHTTPProxyHandler struct {
	Router             *chi.Mux
	authenicator       middleware.Authenticator
	jujuManager        JujuManager
	loginTokenProvider LoginTokenProvider
}

// NewMigrationHTTPProxyHandler creates a model migration proxy http handler.
func NewMigrationHTTPProxyHandler(authenticator middleware.Authenticator, jujuManager JujuManager, loginTokenProvider LoginTokenProvider) *MigrationHTTPProxyHandler {
	return &MigrationHTTPProxyHandler{
		Router:             chi.NewRouter(),
		authenicator:       authenticator,
		jujuManager:        jujuManager,
		loginTokenProvider: loginTokenProvider,
	}
}

// Routes returns the grouped routers routes with group specific middlewares.
func (hph *MigrationHTTPProxyHandler) Routes() chi.Router {
	hph.SetupMiddleware()
	hph.Router.HandleFunc("/charms/*", hph.ProxyHTTP)
	hph.Router.HandleFunc("/tools", hph.ProxyHTTP)
	hph.Router.HandleFunc("/resources", hph.ProxyHTTP)
	return hph.Router
}

// SetupMiddleware applies authn and authz middlewares.
func (hph *MigrationHTTPProxyHandler) SetupMiddleware() {
	hph.Router.Use(func(h http.Handler) http.Handler {
		return middleware.AuthenticateViaBasicAuth(h, hph.authenicator)
	})
	hph.Router.Use(middleware.AuthorizeUserAsJIMMAdmin)
}

// ProxyHTTP extracts the model uuid from an HTTP header to proxy the request to the right controller.
func (hph *MigrationHTTPProxyHandler) ProxyHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	zapctx.Debug(ctx, "starting migration proxy request", zap.String("path", req.URL.Path))

	modelUUID := req.Header.Get(jujuparams.MigrationModelHTTPHeader)
	if modelUUID == "" {
		errMsg := fmt.Sprintf("missing %s header value", jujuparams.MigrationModelHTTPHeader)
		writeError(ctx, w, http.StatusBadRequest, errors.New(errMsg), errMsg)
		return
	}

	user, err := middleware.IdentityFromContext(ctx)
	if err != nil {
		writeError(ctx, w, http.StatusUnauthorized, err, "failed to get authenticated user")
		return
	}

	controllerDetails, err := hph.jujuManager.ControllerDetailsForIncomingModel(ctx, modelUUID)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			writeError(ctx, w, http.StatusNotFound, err, "migrating model not found")
			return
		}
		writeError(ctx, w, http.StatusInternalServerError, err, "cannot retrieve controller")
		return
	}

	// Mint a superuser token since the user is already authorized as a JIMM admin.
	mt := names.NewModelTag(modelUUID)
	ct := names.NewControllerTag(controllerDetails.ControllerUUID)
	jwt, err := hph.loginTokenProvider.NewSuperuserLoginToken(ctx, mt, ct, user)
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
