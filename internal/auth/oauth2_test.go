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
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
)

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

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:      "http://localhost:8082/realms/jimm",
		DeviceClientID: "jimm-device",
		DeviceScopes:   []string{oidc.ScopeOpenID, "profile", "email"},
	})
	c.Assert(err, qt.IsNil)

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

// TestAccessTokens tests both the minting and validation of JIMM
// access tokens.
func TestAccessTokens(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:         "http://localhost:8082/realms/jimm",
		DeviceClientID:    "jimm-device",
		DeviceScopes:      []string{oidc.ScopeOpenID, "profile", "email"},
		AccessTokenExpiry: time.Hour,
	})
	c.Assert(err, qt.IsNil)

	secretKey := "secret-key"
	token, err := authSvc.MintAccessToken("jimm-test@canonical.com", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	jwtToken, err := authSvc.VerifyAccessToken(token, secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "jimm-test@canonical.com")
}

func TestAccessTokenRejectsWrongSecretKey(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:         "http://localhost:8082/realms/jimm",
		DeviceClientID:    "jimm-device",
		DeviceScopes:      []string{oidc.ScopeOpenID, "profile", "email"},
		AccessTokenExpiry: time.Hour,
	})
	c.Assert(err, qt.IsNil)

	secretKey := "secret-key"
	token, err := authSvc.MintAccessToken("jimm-test@canonical.com", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifyAccessToken(token, "wrong key")
	c.Assert(err, qt.ErrorMatches, "could not verify message using any of the signatures or keys")
}

func TestAccessTokenRejectsExpiredToken(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	noDuration := time.Duration(0)

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:         "http://localhost:8082/realms/jimm",
		DeviceClientID:    "jimm-device",
		DeviceScopes:      []string{oidc.ScopeOpenID, "profile", "email"},
		AccessTokenExpiry: noDuration,
	})
	c.Assert(err, qt.IsNil)

	secretKey := "secret-key"
	token, err := authSvc.MintAccessToken("jimm-test@canonical.com", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifyAccessToken(token, secretKey)
	c.Assert(err, qt.ErrorMatches, `"exp" not satisfied`)
}

func TestAccessTokenValidatesEmail(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:         "http://localhost:8082/realms/jimm",
		DeviceClientID:    "jimm-device",
		DeviceScopes:      []string{oidc.ScopeOpenID, "profile", "email"},
		AccessTokenExpiry: time.Hour,
	})
	c.Assert(err, qt.IsNil)

	secretKey := "secret-key"
	token, err := authSvc.MintAccessToken("", secretKey)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifyAccessToken(token, secretKey)
	c.Assert(err, qt.ErrorMatches, "failed to parse email")
}
