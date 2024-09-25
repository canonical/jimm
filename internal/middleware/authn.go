// Copyright 2024 Canonical.

package middleware

import (
	"context"
	"net/http"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// identityContextKey is the unique key to extract user from context for basic-auth authentication
type identityContextKey struct{}

// JIMMAuthner is an interface that requires authentication methods from JIMM.
type JIMMAuthner interface {
	AuthenticateBrowserSession(context.Context, http.ResponseWriter, *http.Request) (context.Context, error)
	LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error)
	UserLogin(ctx context.Context, identityName string) (*openfga.User, error)
}

// AuthenticateViaCookie performs browser session authentication and puts an identity in the request's context
func AuthenticateViaCookie(next http.Handler, jimm JIMMAuthner) http.Handler {
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

// AuthenticateRebac is a layer on top of AuthenticateViaCookie
// It places the OpenFGA user for the session identity inside the request's context
// and verifies that the user is a JIMM admin.
func AuthenticateRebac(next http.Handler, jimm JIMMAuthner) http.Handler {
	return AuthenticateViaCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}

// AuthenticateWithSessionTokenViaBasicAuth performs basic auth authentication and puts an identity in the request's context.
// The basic-auth is composed of an empty user, and as a password a jwt token that we parse and use to authenticate the user.
func AuthenticateWithSessionTokenViaBasicAuth(next http.Handler, jimm JIMMAuthner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// extract auth token
		_, password, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("authentication missing"))
			return
		}
		user, err := jimm.LoginWithSessionToken(ctx, password)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("error authenticating the user"))
			return
		}
		next.ServeHTTP(w, r.WithContext(withIdentity(ctx, user)))
	})
}

// IdentityFromContext extracts the user from the context.
func IdentityFromContext(ctx context.Context) (*openfga.User, error) {
	identity := ctx.Value(identityContextKey{})
	user, ok := identity.(*openfga.User)
	if !ok {
		return nil, errors.E("cannot extract user from context")
	}
	return user, nil
}

// withIdentity sets the user into the context and return the context
func withIdentity(ctx context.Context, user *openfga.User) context.Context {
	return context.WithValue(ctx, identityContextKey{}, user)
}
