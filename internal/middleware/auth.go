// Copyright 2024 Canonical Ltd.

package middleware

import (
	"net/http"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/jimm"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

// AuthenticateViaCookie performs browser session authentication and puts an identity in the request's context
func AuthenticateViaCookie(next http.Handler, jimm *jimm.JIMM) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := jimm.OAuthAuthenticator.AuthenticateBrowserSession(r.Context(), w, r)
		if err != nil {
			zapctx.Error(ctx, "failed to authenticate", zap.Error(err))
			http.Error(w, "failed to authenticate", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthenticateRebac is a layer on top of AuthenticateViaCookie
// It places the OpenFGA user for the session identity inside the request's context.
func AuthenticateRebac(next http.Handler, jimm *jimm.JIMM) http.Handler {
	return AuthenticateViaCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		identity := auth.SessionIdentityFromContext(ctx)
		if identity == "" {
			zapctx.Error(ctx, "no identity found in session")
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}

		user, err := jimm.GetOpenFGAUserAndAuthorise(ctx, identity)
		if err != nil {
			zapctx.Error(ctx, "failed to get openfga user", zap.Error(err))
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}

		ctx = rebac_handlers.ContextWithIdentity(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}), jimm)
}
