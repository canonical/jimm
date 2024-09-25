// Copyright 2024 Canonical.

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

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// These consts holds the endpoint paths for OAuth2.0 related auth.
// AuthResourceBasePath forms the base path and the remainder are
// appended onto the base in practice.
const (
	AuthResourceBasePath = "/auth"
	CallbackEndpoint     = "/callback"
	WhoAmIEndpoint       = "/whoami"
	LogOutEndpoint       = "/logout"
	LoginEndpoint        = "/login"
)

// OAuthHandler handles the oauth2.0 browser flow for JIMM.
// Implements jimmhttp.JIMMHttpHandler.
type OAuthHandler struct {
	Router                    *chi.Mux
	authenticator             BrowserOAuthAuthenticator
	dashboardFinalRedirectURL string
}

// OAuthHandlerParams holds the parameters to configure the OAuthHandler.
type OAuthHandlerParams struct {
	// Authenticator is the authenticator to handle browser authentication.
	Authenticator BrowserOAuthAuthenticator

	// DashboardFinalRedirectURL is the final redirection URL to send users to
	// upon completing the authorisation code flow.
	DashboardFinalRedirectURL string
}

// BrowserOAuthAuthenticator handles authorisation code authentication within JIMM
// via OIDC.
type BrowserOAuthAuthenticator interface {
	AuthCodeURL() (string, string, error)
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error)
	Email(idToken *oidc.IDToken) (string, error)
	UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error
	CreateBrowserSession(
		ctx context.Context,
		w http.ResponseWriter,
		r *http.Request,
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
	}, nil
}

// Routes returns the grouped routers routes with group specific middlewares.
func (oah *OAuthHandler) Routes() chi.Router {
	oah.SetupMiddleware()
	oah.Router.Get(LoginEndpoint, oah.Login)
	oah.Router.Get(CallbackEndpoint, oah.Callback)
	oah.Router.Get(LogOutEndpoint, oah.Logout)
	oah.Router.Get(WhoAmIEndpoint, oah.Whoami)
	return oah.Router
}

// SetupMiddleware applies middlewares.
func (oah *OAuthHandler) SetupMiddleware() {
}

// Login handles /auth/login.
func (oah *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	redirectURL, state, err := oah.authenticator.AuthCodeURL()
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to generate auth redirect URL")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.StateKey,
		Value:    state,
		MaxAge:   900,                                     // 15 min.
		Path:     AuthResourceBasePath + CallbackEndpoint, // Only send the cookie back on /auth paths.
		HttpOnly: true,                                    // Restrict access from JS.
		SameSite: http.SameSiteLaxMode,                    // Allow the cookie to be sent on a redirect from the IdP to JIMM.
	})
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// Callback handles /auth/callback.
func (oah *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stateByCookie, err := r.Cookie(auth.StateKey)
	if err != nil {
		usrErr := errors.E("no state cookie present")
		writeError(ctx, w, http.StatusForbidden, usrErr, "no state cookie present")
		return
	}
	stateByURL := r.URL.Query().Get("state")
	if stateByCookie.Value != stateByURL {
		err := errors.E("state does not match")
		writeError(ctx, w, http.StatusForbidden, err, "state does not match")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		err := errors.E("missing auth code")
		writeError(ctx, w, http.StatusForbidden, err, "no authorisation code present")
		return
	}

	authSvc := oah.authenticator

	token, err := authSvc.Exchange(ctx, code)
	if err != nil {
		writeError(ctx, w, http.StatusForbidden, err, "failed to exchange authcode")
		return
	}

	idToken, err := authSvc.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to extract and verify id token")
		return
	}

	email, err := authSvc.Email(idToken)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to extract email from id token")
		return
	}

	if err := authSvc.UpdateIdentity(ctx, email, token); err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to update identity")
		return
	}

	if err := oah.authenticator.CreateBrowserSession(
		ctx,
		w,
		r,
		email,
	); err != nil {
		writeError(ctx, w, http.StatusInternalServerError, err, "failed to setup session")
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
		if errors.ErrorCode(err) == errors.CodeForbidden {
			w.WriteHeader(http.StatusForbidden)
			return
		}
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

	w.Header().Add("Content-Type", "application/json")
	if _, err := w.Write(b); err != nil {
		zapctx.Error(ctx, "failed to write whoami body", zap.Error(err))
	}
}

// writeError writes an error and logs the message. It is expected that the status code
// is an erroneous status code.
func writeError(ctx context.Context, w http.ResponseWriter, status int, err error, logMessage string) {
	zapctx.Error(ctx, logMessage, zap.Error(err))
	w.WriteHeader(status)
	errMsg := ""
	if err != nil {
		errMsg = " - " + err.Error()
	}
	_, err = w.Write([]byte(http.StatusText(status) + errMsg))
	if err != nil {
		zapctx.Error(ctx, "failed to write status text error", zap.Error(err))
	}
}
