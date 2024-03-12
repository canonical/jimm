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

var (
	jwtTestSecret = "test-secret"
)

// A SimpleTester is a simple version of the test interface
// that both the GoChecker and QuickTest checker satisfy.
// Useful for enabling test setup functions to fail without a panic.
type SimpleTester interface {
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

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

// NewUserSessionLogin returns a login provider than be used with Juju Dial Opts
// to define how login will take place. In this case we login using a session token
// that the JIMM server should verify with the same test secret.
func NewUserSessionLogin(c SimpleTester, username string) api.LoginProvider {
	email := convertUsernameToEmail(username)
	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(1 * time.Hour)).
		Build()
	if err != nil {
		c.Fatalf("failed to generate test session token")
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(jwtTestSecret)))
	if err != nil {
		c.Fatalf("failed to sign test session token")
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
