// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/openfga"
)

const (
	JWTTestSecret = "test-secret"
)

// An Authenticator is an implementation of jimm.Authenticator that returns
// the stored user and error.
type Authenticator struct {
	User *openfga.User
	Err  error
}

// Authenticate implements jimm.Authenticator.
func (a Authenticator) Authenticate(_ context.Context, _ *jujuparams.LoginRequest) (*openfga.User, error) {
	return a.User, a.Err
}

type MockOAuthAuthenticator struct {
	jimm.OAuthAuthenticator
	secretKey string
}

func NewMockOAuthAuthenticator(secretKey string) MockOAuthAuthenticator {
	return MockOAuthAuthenticator{secretKey: secretKey}
}

// VerifySessionToken provides the mock implementation for verifying session tokens.
// Allowing JIMM tests to create their own session tokens that will always be accepted.
func (m MockOAuthAuthenticator) VerifySessionToken(token string, secretKey string) (jwt.Token, error) {
	return auth.VerifySessionToken(token, m.secretKey)
}

func NewUserSessionLogin(username string) api.LoginProvider {
	email := convertUsernameToEmail(username)
	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(1 * time.Hour)).
		Build()
	if err != nil {
		panic("failed to generate test session token")
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(JWTTestSecret)))
	if err != nil {
		panic("failed to sign test session token")
	}

	b64Token := base64.StdEncoding.EncodeToString(freshToken)
	lp := api.NewSessionTokenLoginProvider(b64Token, nil, nil)
	return lp
}

func convertUsernameToEmail(username string) string {
	if !strings.Contains(username, "@") {
		return username + "@canonical.com"
	}
	return username
}
