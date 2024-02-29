package jimmhttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// OAuthHandler handles the oauth2.0 browser flow for JIMM.
// Implements jimmhttp.JIMMHttpHandler.
type OAuthHandler struct {
	Router        *chi.Mux
	Authenticator BrowserOAuthAuthenticator
}

// BrowserOAuthAuthenticator handles authorisation code authentication within JIMM
// via OIDC.
type BrowserOAuthAuthenticator interface {
	// AuthCodeURL returns a URL that will be used to redirect a browser to the identity provider.
	AuthCodeURL() string
}

// NewOAuthHandler returns a new OAuth handler.
func NewOAuthHandler(authenticator BrowserOAuthAuthenticator) *OAuthHandler {
	return &OAuthHandler{Router: chi.NewRouter(), Authenticator: authenticator}
}

// Routes returns the grouped routers routes with group specific middlewares.
func (oah *OAuthHandler) Routes() chi.Router {
	oah.SetupMiddleware()
	oah.Router.Get("/login", oah.Login)
	return oah.Router
}

// SetupMiddleware applies middlewares.
func (oah *OAuthHandler) SetupMiddleware() {
}

// Login handles /auth/login,
func (oah *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	redirectURL := oah.Authenticator.AuthCodeURL()
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}
