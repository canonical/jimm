package jimmhttp

import (
	"context"
	"net/http"

	"github.com/antonlindstrom/pgstore"
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
	authenticator             BrowserOAuthAuthenticator
	dashboardFinalRedirectURL string
	sessionStore              *pgstore.PGStore
	secureCookies             bool
	cookieExpiry              int
}

type OAuthHandlerParams struct {
	// Authenticator is the authenticator to handle browser authentication.
	Authenticator BrowserOAuthAuthenticator

	// DashboardFinalRedirectURL is the final redirection URL to send users to
	// upon completing the authorisation code flow.
	DashboardFinalRedirectURL string

	// SessionStore is the cookie session store.
	SessionStore *pgstore.PGStore

	// SessionCookies determines if HTTPS must be enabled in order for JIMM
	// to set cookies when creating browser based sessions.
	SecureCookies bool

	// CookieExpiry is how long the cookie will be valid before expiring in seconds.
	CookieExpiry int
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
func NewOAuthHandler(p OAuthHandlerParams) (*OAuthHandler, error) {
	if p.Authenticator == nil {
		return nil, errors.E("nil authenticator")
	}
	if p.DashboardFinalRedirectURL == "" {
		return nil, errors.E("final redirect url not specified")
	}
	if p.SessionStore == nil {
		return nil, errors.E("nil session store")
	}
	return &OAuthHandler{
		Router:                    chi.NewRouter(),
		authenticator:             p.Authenticator,
		dashboardFinalRedirectURL: p.DashboardFinalRedirectURL,
		sessionStore:              p.SessionStore,
		secureCookies:             p.SecureCookies,
		cookieExpiry:              p.CookieExpiry,
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
	redirectURL := oah.authenticator.AuthCodeURL()
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// Callback handles /auth/callback.
func (oah *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

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

	// If the session is empty, it'll just be an empty session, we only check
	// errors for bad decoding etc.
	session, err := oah.sessionStore.Get(r, "jimm-browser-session")
	if err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to get session")
	}

	session.IsNew = true                       // Sets cookie to a fresh new cookie
	session.Options.MaxAge = oah.cookieExpiry  // 24 Hours expiry
	session.Options.Secure = oah.secureCookies // Ensures only sent with HTTPS
	session.Options.HttpOnly = false           // Allow Javascript to read it

	session.Values["jimm-session"] = email
	if err = session.Save(r, w); err != nil {
		writeError(ctx, w, http.StatusBadRequest, err, "failed to save session")
	}
	http.Redirect(w, r, oah.dashboardFinalRedirectURL, http.StatusPermanentRedirect)
}

// writeError writes an error and logs the message. It is expected that the status code
// is an erroneous status code.
func writeError(ctx context.Context, w http.ResponseWriter, status int, err error, logMessage string) {
	zapctx.Error(ctx, logMessage, zap.Error(err))
	w.WriteHeader(status)
	w.Write([]byte(http.StatusText(status)))
}
