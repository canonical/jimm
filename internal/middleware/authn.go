// Copyright 2024 Canonical.

package middleware

import (
	"context"
	"net/http"
	"strings"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// identityContextKey is the unique key to extract user from context for basic-auth authentication
type identityContextKey struct{}

// Authenticator is an interface that requires authentication methods from JIMM.
type Authenticator interface {
	AuthenticateBrowserSession(context.Context, http.ResponseWriter, *http.Request) (context.Context, error)
	LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error)
	UserLogin(ctx context.Context, identityName string) (*openfga.User, error)
}

// AuthenticateViaCookie performs browser session authentication and puts an identity in the request's context
func AuthenticateViaCookie(next http.Handler, jimm Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := jimm.AuthenticateBrowserSession(r.Context(), w, r)
		if err != nil {
			zapctx.Error(ctx, "failed to authenticate", zap.Error(err))
			http.Error(w, "failed to authenticate", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Set of ReBAC Admin API endpoints that do not require authentication.
var unauthenticatedEndpoints = map[string]struct{}{
	"/v1/swagger.json": {},
}

// AuthenticateRebac is a layer on top of AuthenticateViaCookie. It places the
// OpenFGA user for the session identity inside the request's context and
// verifies that the user is a JIMM admin. Note that the method needs the base
// URL to decide if the request does not require authentication; this is to
// safeguard against conflicting/similar endpoint names in the future.
func AuthenticateRebac(baseURL string, next http.Handler, jimm Authenticator) http.Handler {
	cookieAuthenticator := AuthenticateViaCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		identity := auth.SessionIdentityFromContext(ctx)
		if identity == "" {
			zapctx.Error(ctx, "no identity found in session")
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}

		user, err := jimm.UserLogin(ctx, identity)
		if err != nil {
			zapctx.Error(ctx, "failed to get openfga user", zap.Error(err))
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}
		if !user.JimmAdmin {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("user is not an admin"))
			return
		}

		ctx = rebac_handlers.ContextWithIdentity(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}), jimm)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relativePath, _ := strings.CutPrefix(r.URL.Path, baseURL)
		if _, found := unauthenticatedEndpoints[relativePath]; found {
			next.ServeHTTP(w, r)
			return
		}
		cookieAuthenticator.ServeHTTP(w, r)
	})
}

// AuthenticateViaBasicAuth performs basic auth authentication and puts an identity in the request's context.
// For basic auth, we support two modes:
// 1. Client Credentials: where the username is the client ID, and the password is the client secret.
// 2. Session Token: where the username is empty, and the password is a session token.
func AuthenticateViaBasicAuth(next http.Handler, jimm Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// extract auth token
		username, password, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("authentication missing"))
			return
		}

		// if username is set, we assume client credentials authentication
		if username != "" {
			// then try client credentials authentication
			user, err := jimm.LoginClientCredentials(ctx, username, password)
			if err == nil {
				next.ServeHTTP(w, r.WithContext(withIdentity(ctx, user)))
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("error authenticating the user"))
			return
		}

		user, err := jimm.LoginWithSessionToken(ctx, password)
		if err == nil {
			next.ServeHTTP(w, r.WithContext(withIdentity(ctx, user)))
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("error authenticating the user"))
	})
}

// IdentityFromContext extracts the user from the context.
func IdentityFromContext(ctx context.Context) (*openfga.User, error) {
	identity := ctx.Value(identityContextKey{})
	user, ok := identity.(*openfga.User)
	if !ok {
		return nil, errors.New("cannot extract user from context")
	}
	return user, nil
}

// withIdentity sets the user into the context and return the context
func withIdentity(ctx context.Context, user *openfga.User) context.Context {
	return context.WithValue(ctx, identityContextKey{}, user)
}
