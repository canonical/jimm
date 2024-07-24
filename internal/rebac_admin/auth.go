// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"net/http"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/jimm"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

func newAuthenticator(jimm *jimm.JIMM) *authenticator {
	return &authenticator{
		jimm,
	}
}

type dummyWriter struct{}

func (d dummyWriter) Header() http.Header {
	return http.Header{}
}

func (d dummyWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (d dummyWriter) WriteHeader(int) {}

type authenticator struct {
	jimm *jimm.JIMM
}

// Authenticate extracts the calling user's information from the given HTTP request
func (a *authenticator) Authenticate(r *http.Request) (any, error) {
	// AuthenticateBrowserSession modifies cookies in the response writer which isn't present here
	dummyWriter := &dummyWriter{}

	ctx, err := a.jimm.OAuthAuthenticator.AuthenticateBrowserSession(r.Context(), dummyWriter, r)
	if err != nil {
		zapctx.Error(ctx, "failed to authenticate", zap.Error(err))
		return nil, rebac_handlers.NewAuthenticationError("failed to authenticate")
	}

	identity := auth.SessionIdentityFromContext(ctx)
	if identity == "" {
		zapctx.Error(ctx, "no identity found in session")
		return nil, rebac_handlers.NewAuthenticationError("no identity found in session")
	}

	user, err := a.jimm.GetOpenFGAUserAndAuthorise(ctx, identity)
	if err != nil {
		zapctx.Error(ctx, "failed to get openfga user", zap.Error(err))
		return nil, rebac_handlers.NewAuthenticationError("failed to get openfga user")
	}

	return user, nil
}
