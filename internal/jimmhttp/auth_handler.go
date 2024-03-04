package jimmhttp

import (
	"context"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/internal/errors"
)

// OAuthHandler handles the oauth2.0 browser flow for JIMM.
// Implements jimmhttp.JIMMHttpHandler.
type OAuthHandler struct {
	Router                    *chi.Mux
	Authenticator             BrowserOAuthAuthenticator
	DashboardFinalRedirectURL string
}

// BrowserOAuthAuthenticator handles authorisation code authentication within JIMM
// via OIDC.
type BrowserOAuthAuthenticator interface {
	AuthCodeURL() string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error)
	Email(idToken *oidc.IDToken) (string, error)
	UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error
}

// NewOAuthHandler returns a new OAuth handler.
func NewOAuthHandler(authenticator BrowserOAuthAuthenticator, dashboardFinalRedirectURL string) (*OAuthHandler, error) {
	if authenticator == nil {
		return nil, errors.E("nil authenticator")
	}
	if dashboardFinalRedirectURL == "" {
		return nil, errors.E("final redirect url not specified")
	}
	return &OAuthHandler{
		Router:                    chi.NewRouter(),
		Authenticator:             authenticator,
		DashboardFinalRedirectURL: dashboardFinalRedirectURL,
	}, nil
}

// Routes returns the grouped routers routes with group specific middlewares.
func (oah *OAuthHandler) Routes() chi.Router {
	oah.SetupMiddleware()
	oah.Router.Get("/login", oah.Login)
	oah.Router.Get("/callback", oah.Callback)
	return oah.Router
}

// SetupMiddleware applies middlewares.
func (oah *OAuthHandler) SetupMiddleware() {
}

// Login handles /auth/login.
func (oah *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	redirectURL := oah.Authenticator.AuthCodeURL()
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

func write500(ctx context.Context, err error, w http.ResponseWriter, logMsg string) {
	zapctx.Error(ctx, logMsg, zap.Error(err))
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Internal Server Error."))
}

// Callback handles /auth/callback.
func (oah *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	code := r.URL.Query().Get("code")

	authSvc := oah.Authenticator

	token, err := authSvc.Exchange(ctx, code)
	if err != nil {
		write500(ctx, err, w, "failed to exchange authcode")
		return
	}

	idToken, err := authSvc.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		write500(ctx, err, w, "failed to extract and verify id token")
		return
	}

	email, err := authSvc.Email(idToken)
	if err != nil {
		write500(ctx, err, w, "failed to extract email from id token")
		return
	}

	if err := authSvc.UpdateIdentity(ctx, email, token); err != nil {
		write500(ctx, err, w, "failed to update identity")
		return
	}

	http.Redirect(w, r, oah.DashboardFinalRedirectURL, http.StatusPermanentRedirect)
}
