// Copyright 2025 Canonical.

package jimmhttp

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	jujuparams "github.com/juju/juju/rpc/params"
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
	Router          *chi.Mux
	authenicator    middleware.Authenticator
	credentialStore CredentialStore
}

// NewMigrationHTTPProxyHandler creates a model migration proxy http handler.
func NewMigrationHTTPProxyHandler(authenticator middleware.Authenticator, credentialStore CredentialStore) *MigrationHTTPProxyHandler {
	return &MigrationHTTPProxyHandler{
		Router:          chi.NewRouter(),
		authenicator:    authenticator,
		credentialStore: credentialStore,
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
		writeError(ctx, w, http.StatusBadRequest, errors.E(errMsg), errMsg)
		return
	}

	controllerDetails, err := hph.credentialStore.ControllerDetailsForIncomingModel(ctx, modelUUID)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			writeError(ctx, w, http.StatusNotFound, err, "migrating model not found")
			return
		}
		writeError(ctx, w, http.StatusInternalServerError, err, "cannot retrieve controller")
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
