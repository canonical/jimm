// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"net/http"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// streamLoginManager authenticates identities used to access streamed Juju
// endpoints.
type streamLoginManager interface {
	LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error)
}

// authenticateStreamBasicAuth authenticates either of the Basic authentication
// schemes supported by Juju clients. An empty username indicates a JIMM session
// token; a non-empty username and password are client credentials.
func authenticateStreamBasicAuth(ctx context.Context, req *http.Request, loginManager streamLoginManager) (context.Context, error) {
	// Although the authentication details are sent via basic auth headers, they are not a username/password.
	// To understand why - the Juju client incorrectly used basic auth instead of bearer auth when sending
	// a session token. When performing a client-credential login, we incorrectly send the client id/secret
	// directly to JIMM instead of having the client exchange them for a token with the identity provider.
	//
	// We keep this behavior for backwards compatibility with the Juju client, but it is not a standard use of basic auth.
	username, password, ok := req.BasicAuth()
	if !ok {
		return ctx, errors.Codef(errors.CodeUnauthorized, "authentication missing")
	}

	var (
		user *openfga.User
		err  error
	)
	if username == "" {
		user, err = loginManager.LoginWithSessionToken(ctx, password)
	} else {
		user, err = loginManager.LoginClientCredentials(ctx, username, password)
	}
	if err != nil {
		return ctx, errors.Codef(errors.CodeUnauthorized, "%w", err)
	}

	return middleware.ContextWithIdentity(ctx, user), nil
}
