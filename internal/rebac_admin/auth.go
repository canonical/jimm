// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"net/http"
	"net/http/httptest"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/jimm"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

type authenticator struct {
	jimm *jimm.JIMM
}

func newAuthenticator(jimm *jimm.JIMM) *authenticator {
	return &authenticator{
		jimm,
	}
}

// Authenticate extracts the calling user's information from the given HTTP request
func (a *authenticator) Authenticate(r *http.Request) (any, error) {
	// AuthenticateBrowserSession modifies cookies in the response writer which isn't present here
	dummyWriter := httptest.NewRecorder()

	ctx, err := a.jimm.OAuthAuthenticator.AuthenticateBrowserSession(r.Context(), dummyWriter, r)
	if err != nil {
		return nil, rebac_handlers.NewAuthenticationError("failed to authenticate")
	}

	identity := auth.SessionIdentityFromContext(ctx)
	if identity == "" {
		return nil, rebac_handlers.NewAuthenticationError("no identity found in session")
	}

	user, err := a.jimm.GetOpenFGAUserAndAuthorise(ctx, identity)
	if err != nil {
		return nil, rebac_handlers.NewAuthenticationError("failed to get openfga user")
	}

	return user, nil
}
