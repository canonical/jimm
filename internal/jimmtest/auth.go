// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/internal/openfga"
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
}

func (m MockOAuthAuthenticator) Device(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	return nil, nil
}

func (m MockOAuthAuthenticator) DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	return nil, nil
}

func (m MockOAuthAuthenticator) ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error) {
	return nil, nil
}

func (m MockOAuthAuthenticator) Email(idToken *oidc.IDToken) (string, error) {
	return "", nil
}

func (m MockOAuthAuthenticator) MintSessionToken(email string, secretKey string) (string, error) {
	return "", nil
}

func (m MockOAuthAuthenticator) VerifySessionToken(token string, secretKey string) (jwt.Token, error) {
	return nil, nil
}
