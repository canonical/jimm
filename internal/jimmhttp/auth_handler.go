package jimmhttp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/errors"
)

// OAuthHandler handles the oauth2.0 browser flow for JIMM.
// Implements jimmhttp.JIMMHttpHandler.
type OAuthHandler struct {
	Router                    *chi.Mux
	authenticator             BrowserOAuthAuthenticator
	dashboardFinalRedirectURL string
	secureCookies             bool
}

// OAuthHandlerParams holds the parameters to configure the OAuthHandler.
type OAuthHandlerParams struct {
	// Authenticator is the authenticator to handle browser authentication.
	Authenticator BrowserOAuthAuthenticator

	// DashboardFinalRedirectURL is the final redirection URL to send users to
	// upon completing the authorisation code flow.
	DashboardFinalRedirectURL string

	// SessionCookies determines if HTTPS must be enabled in order for JIMM
	// to set cookies when creating browser based sessions.
	SecureCookies bool
}

// BrowserOAuthAuthenticator handles authorisation code authentication within JIMM
// via OIDC.
type BrowserOAuthAuthenticator interface {
	AuthCodeURL() string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error)
	Email(idToken *oidc.IDToken) (string, error)
	UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error
	CreateBrowserSession(
		ctx context.Context,
		w http.ResponseWriter,
		r *http.Request,
		secureCookies bool,
		email string,
	) error
	Logout(ctx context.Context, w http.ResponseWriter, req *http.Request) error
	AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
	Whoami(ctx context.Context) (*params.WhoamiResponse, error)
}

// NewOAuthHandler returns a new OAuth handler.
func NewOAuthHandler(p OAuthHandlerParams) (*OAuthHandler, error) {
	if p.Authenticator == nil {
		return nil, errors.E("nil authenticator")
	}
	if p.DashboardFinalRedirectURL == "" {
		return nil, errors.E("final redirect url not specified")
	}
	return &OAuthHandler{
		Router:                    chi.NewRouter(),
		authenticator:             p.Authenticator,
		dashboardFinalRedirectURL: p.DashboardFinalRedirectURL,
		secureCookies:             p.SecureCookies,
	}, nil
}

// Routes returns the grouped routers routes with group specific middlewares.
func (oah *OAuthHandler) Routes() chi.Router {
	oah.SetupMiddleware()
	oah.Router.Get("/login", oah.Login)
	oah.Router.Get("/callback", oah.Callback)
	oah.Router.Get("/logout", oah.Logout)
	oah.Router.Get("/whoami", oah.Whoami)
	return oah.Router
}

// SetupMiddleware applies middlewares.
func (oah *OAuthHandler) SetupMiddleware() {
}

// Login handles /auth/login.
func (oah *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	redirectURL := oah.authenticator.AuthCodeURL()
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// Callback handles /auth/callback.
func (oah *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(ctx, w, http.StatusBadRequest, nil, "no authorisation code present")
		return
	}

	authSvc := oah.authenticator

	token, err := authSvc.Exchange(ctx, code)
	if err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to exchange authcode")
		return
	}

	idToken, err := authSvc.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to extract and verify id token")
		return
	}

	email, err := authSvc.Email(idToken)
	if err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to extract email from id token")
		return
	}

	if err := authSvc.UpdateIdentity(ctx, email, token); err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to update identity")
		return
	}

	if err := oah.authenticator.CreateBrowserSession(
		ctx,
		w,
		r,
		oah.secureCookies,
		email,
	); err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to setup session")
	}

	http.Redirect(w, r, oah.dashboardFinalRedirectURL, http.StatusPermanentRedirect)
}

// Logout handles /auth/logout.
func (oah *OAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	authSvc := oah.authenticator

	if _, err := r.Cookie(auth.SessionName); err != nil {
		writeError(ctx, w, http.StatusForbidden, err, "no session cookie to logout")
		return
	}

	if err := authSvc.Logout(ctx, w, r); err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to logout")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Whoami handles /auth/whoami.
func (oah *OAuthHandler) Whoami(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	authSvc := oah.authenticator

	if _, err := r.Cookie(auth.SessionName); err != nil {
		writeError(ctx, w, http.StatusForbidden, err, "no session cookie to identity user")
		return
	}

	ctx, err := authSvc.AuthenticateBrowserSession(ctx, w, r)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to authenticate users session")
		return
	}

	whoamiResp, err := authSvc.Whoami(ctx)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to find whoami from identity id")
		return
	}

	b, err := json.Marshal(whoamiResp)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to marshal whoami resp")
		return
	}

	if _, err := w.Write(b); err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to write response to whoami")
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// writeError writes an error and logs the message. It is expected that the status code
// is an erroneous status code.
func writeError(ctx context.Context, w http.ResponseWriter, status int, err error, logMessage string) {
	zapctx.Error(ctx, logMessage, zap.Error(err))
	w.WriteHeader(status)
	w.Write([]byte(http.StatusText(status)))
}
