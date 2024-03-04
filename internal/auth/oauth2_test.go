// Copyright 2024 canonical.

package auth_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
)

func setupTestAuthSvc(ctx context.Context, c *qt.C, expiry time.Duration) *auth.AuthenticationService {
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		ClientID:           "jimm-device",
		ClientSecret:       "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:             []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry: expiry,
		RedirectURL:        "http://localhost:8080/auth/callback",
		Db: &db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return time.Now() }),
		},
	})
	c.Assert(err, qt.IsNil)

	return authSvc
}

// This test requires the local docker compose to be running and keycloak
// to be available.
//
// TODO(ale8k): Use a mock for this and also device below, but future work???
func TestAuthCodeURL(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc := setupTestAuthSvc(ctx, c, time.Hour)

	url := authSvc.AuthCodeURL()
	c.Assert(
		url,
		qt.Equals,
		`http://localhost:8082/realms/jimm/protocol/openid-connect/auth?client_id=jimm-device&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fauth%2Fcallback&response_type=code&scope=openid+profile+email`,
	)
}

// TestDevice is a unique test in that it runs through the entire device oauth2.0
// flow and additionally ensures the id token is verified and correct.
//
// This test requires the local docker compose to be running and keycloak
// to be available.
//
// Some calls perform regexes against the response HTML forms such that we
// can manually POST the forms throughout the flow.
func TestDevice(t *testing.T) {
	c := qt.New(t)

	u, err := jimmtest.CreateRandomKeycloakUser()
	c.Assert(err, qt.IsNil)

	ctx := context.Background()

	authSvc := setupTestAuthSvc(ctx, c, time.Hour)

	res, err := authSvc.Device(ctx)
	c.Assert(err, qt.IsNil)

	jar, err := cookiejar.New(nil)
	c.Assert(err, qt.IsNil)

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Println("redirected to", req.URL)
			return nil
		},
	}

	// Post login form
	verResp, err := client.Get(res.VerificationURIComplete)
	c.Assert(err, qt.IsNil)
	defer verResp.Body.Close()
	b, err := io.ReadAll(verResp.Body)
	c.Assert(err, qt.IsNil)

	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(string(b))
	loginFormUrl := match[1]

	v := url.Values{}
	v.Add("username", u.Username)
	v.Add("password", u.Password)
	loginResp, err := client.PostForm(loginFormUrl, v)
	c.Assert(err, qt.IsNil)
	defer loginResp.Body.Close()

	// Post consent
	b, err = io.ReadAll(loginResp.Body)
	c.Assert(err, qt.IsNil)

	re = regexp.MustCompile(`action="(.*?)" method=`)
	match = re.FindStringSubmatch(string(b))
	consentFormUri := match[1]
	v = url.Values{}
	v.Add("accept", "Yes")
	consentResp, err := client.PostForm("http://localhost:8082"+consentFormUri, v)
	c.Assert(err, qt.IsNil)
	defer consentResp.Body.Close()

	// Read consent resp
	b, err = io.ReadAll(consentResp.Body)
	c.Assert(err, qt.IsNil)

	re = regexp.MustCompile(`Device Login Successful`)
	c.Assert(re.MatchString(string(b)), qt.IsTrue)

	// Retrieve access token
	token, err := authSvc.DeviceAccessToken(ctx, res)
	c.Assert(err, qt.IsNil)
	c.Assert(token, qt.IsNotNil)

	// Extract and verify id token
	idToken, err := authSvc.ExtractAndVerifyIDToken(ctx, token)
	c.Assert(err, qt.IsNil)
	c.Assert(idToken, qt.IsNotNil)

	// Test subject set
	c.Assert(idToken.Subject, qt.Equals, u.Id)

	// Retrieve the email
	email, err := authSvc.Email(idToken)
	c.Assert(err, qt.IsNil)
	c.Assert(email, qt.Equals, u.Email)
}

// TestSessionTokens tests both the minting and validation of JIMM
// session tokens.
func TestSessionTokens(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc := setupTestAuthSvc(ctx, c, time.Hour)

	secretKey := "secret-key"
	token, err := authSvc.MintSessionToken("jimm-test@canonical.com", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	jwtToken, err := authSvc.VerifySessionToken(token, secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "jimm-test@canonical.com")
}

func TestSessionTokenRejectsWrongSecretKey(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc := setupTestAuthSvc(ctx, c, time.Hour)

	secretKey := "secret-key"
	token, err := authSvc.MintSessionToken("jimm-test@canonical.com", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifySessionToken(token, "wrong key")
	c.Assert(err, qt.ErrorMatches, "could not verify message using any of the signatures or keys")
}

func TestSessionTokenRejectsExpiredToken(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	noDuration := time.Duration(0)
	authSvc := setupTestAuthSvc(ctx, c, noDuration)

	secretKey := "secret-key"
	token, err := authSvc.MintSessionToken("jimm-test@canonical.com", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifySessionToken(token, secretKey)
	c.Assert(err, qt.ErrorMatches, `JIMM session token expired`)
}

func TestSessionTokenValidatesEmail(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc := setupTestAuthSvc(ctx, c, time.Hour)

	secretKey := "secret-key"
	token, err := authSvc.MintSessionToken("", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifySessionToken(token, secretKey)
	c.Assert(err, qt.ErrorMatches, "failed to parse email")
}
