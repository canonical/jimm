// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"
)

type adminSuite struct {
	websocketSuite
}

func (s *adminSuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)
	ctx := context.Background()
	// Replace JIMM's mock authenticator with a real one here
	// for testing the login flows.
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		ClientID:           "jimm-device",
		ClientSecret:       "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:             []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry: time.Hour,
	})
	c.Assert(err, gc.Equals, nil)
	s.JIMM.OAuthAuthenticator = authSvc
}

var _ = gc.Suite(&adminSuite{})

func (s *adminSuite) TestLoginToController(c *gc.C) {
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.Equals, nil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotImplemented)
}

func (s *adminSuite) TestLoginToControllerWithInvalidMacaroon(c *gc.C) {
	invalidMacaroon, err := macaroon.New(nil, []byte("invalid"), "", macaroon.V1)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		Macaroons: []macaroon.Slice{{invalidMacaroon}},
	}, "test")
	conn.Close()
}

// TestDeviceLogin takes a test user through the flow of logging into jimm
// via the correct facades. All are done in a single test to see the flow end-2-end.
//
// Within the test are clear comments explaining what is happening when and why.
// Please refer to these comments for further details.
func (s *adminSuite) TestDeviceLogin(c *gc.C) {
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()

	// Create a user in keycloak
	user, err := jimmtest.CreateRandomKeycloakUser()
	c.Assert(err, gc.IsNil)

	// We create a http client to keep the same cookies across all requests
	// using a simple jar.
	jar, err := cookiejar.New(nil)
	c.Assert(err, gc.IsNil)

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Println("redirected to", req.URL)
			return nil
		},
	}

	// Step 1, initiate a device login and get the verification URI and usercode.
	// Next, the user will send this code to the verification URI.
	//
	// To simplify the test, we're not going the browser route and instead
	// are going to use the VerificationURIComplete which is equivalent to scanning
	// a QR code. Both are ultimately the same.
	//
	// A normal verification URI looks like: http://localhost:8082/realms/jimm/device
	// in which the user code is posted.
	//
	// A complete URI looks like: http://localhost:8082/realms/jimm/device?user_code=HOKO-OTRV
	// where the user code is set as a part of the query string.
	var loginDeviceResp params.LoginDeviceResponse
	err = conn.APICall("Admin", 4, "", "LoginDevice", nil, &loginDeviceResp)
	c.Assert(err, gc.IsNil)
	c.Assert(loginDeviceResp.UserCode, gc.Not(gc.IsNil))
	c.Assert(loginDeviceResp.VerificationURI, gc.Equals, "http://localhost:8082/realms/jimm/device")

	// Step 2, complete the user side of the authentication by sending the
	// user code to the verification URI using the "complete" method.
	userResp, err := client.Get(loginDeviceResp.VerificationURI + "?user_code=" + loginDeviceResp.UserCode)
	c.Assert(err, gc.IsNil)
	body := userResp.Body
	defer body.Close()
	b, err := io.ReadAll(body)
	c.Assert(err, gc.IsNil)
	loginForm := string(b)

	// Step 2.1, handle the login form (see this func for more details)
	handleLoginForm(c, loginForm, client, user.Username, user.Password)

	// Step 3, after the user has entered the user code, the polling for an access
	// token will complete. The polling can begin before OR after the user has entered the
	// user code, for the simplicity of testing, we are retrieving it AFTER.
	var sessionTokenResp params.GetDeviceSessionTokenResponse
	err = conn.APICall("Admin", 4, "", "GetDeviceSessionToken", nil, &sessionTokenResp)
	c.Assert(err, gc.IsNil)
	// Ensure it is base64 and decodable
	decodedToken, err := base64.StdEncoding.DecodeString(sessionTokenResp.SessionToken)
	c.Assert(err, gc.IsNil)

	// Step 4, use this session token to "login".

	// Test no token present
	var loginResult jujuparams.LoginResult
	err = conn.APICall("Admin", 4, "", "LoginWithSessionToken", nil, &loginResult)
	c.Assert(err, gc.ErrorMatches, "authentication failed, no token presented.*")

	// Test token not base64 encoded
	err = conn.APICall("Admin", 4, "", "LoginWithSessionToken", params.LoginWithSessionTokenRequest{SessionToken: string(decodedToken)}, &loginResult)
	c.Assert(err, gc.ErrorMatches, "authentication failed, failed to decode token.*")

	// Test token base64 encoded passes authentication
	err = conn.APICall("Admin", 4, "", "LoginWithSessionToken", params.LoginWithSessionTokenRequest{SessionToken: sessionTokenResp.SessionToken}, &loginResult)
	c.Assert(err, gc.IsNil)
	c.Assert(loginResult.UserInfo.Identity, gc.Equals, "user-"+user.Email)
	c.Assert(loginResult.UserInfo.DisplayName, gc.Equals, strings.Split(user.Email, "@")[0])

	// Finally, ensure db did indeed update the access token for this user
	updatedUser := &dbmodel.Identity{
		Name: user.Email,
	}
	c.Assert(s.JIMM.DB().GetIdentity(context.Background(), updatedUser), gc.IsNil)
	// TODO(ale8k): Do we need to validate the token again for the test?
	// It has just been through a verifier etc and was returned directly
	// from the device grant?
	c.Assert(updatedUser.AccessToken, gc.Not(gc.Equals), "")

}

// handleLoginForm runs through the login process emulating the user typing in
// their username and password and then clicking consent, to complete
// the device login flow.
func handleLoginForm(c *gc.C, loginForm string, client *http.Client, username, password string) {
	// Step 2.2, now we'll be redirected to a sign-in page and must sign in.
	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(loginForm)
	loginFormUrl := match[1]

	// The username and password are hardcoded witih jimm-realm.json in our local
	// keycloak configuration for the jimm realm.
	v := url.Values{}
	v.Add("username", username)
	v.Add("password", password)
	loginResp, err := client.PostForm(loginFormUrl, v)
	c.Assert(err, gc.IsNil)

	loginRespBody := loginResp.Body
	defer loginRespBody.Close()

	// Step 2.3, the user will now be redirected to a consent screen
	// and is expected to click "yes". We simulate this by posting the form programatically.
	loginRespB, err := io.ReadAll(loginRespBody)
	c.Assert(err, gc.IsNil)
	loginRespS := string(loginRespB)

	re = regexp.MustCompile(`action="(.*?)" method=`)
	match = re.FindStringSubmatch(loginRespS)
	consentFormUri := match[1]

	// We post the "yes" value to accept it.
	v = url.Values{}
	v.Add("accept", "Yes")
	consentResp, err := client.PostForm("http://localhost:8082"+consentFormUri, v)
	c.Assert(err, gc.IsNil)
	defer consentResp.Body.Close()

	// Read the response to ensure it is OK and has been accepted.
	b, err := io.ReadAll(consentResp.Body)
	c.Assert(err, gc.IsNil)

	re = regexp.MustCompile(`Device Login Successful`)
	c.Assert(re.MatchString(string(b)), gc.Equals, true)
}
