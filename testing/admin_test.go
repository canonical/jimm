// Copyright 2025 Canonical.

package testing

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestLoginToController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	conn := s.Open(c, &api.Info{
		SkipLogin: true,
	}, "test@canonical.com", nil)
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, qt.ErrorMatches, `JIMM does not support login from old clients \(not supported\)`)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, qt.ErrorMatches, "(?s).*not implemented.*")
}

// TestBrowserLogin takes a test user through the flow of logging into jimm
// via the correct facades. All are done in a single test to see the flow end-2-end.
//
// Within the test are clear comments explaining what is happening when and why.
// Please refer to these comments for further details.
//
// We only test happy path here due to having tested edge cases and failure cases
// within the auth service itself such as invalid cookies, expired access tokens and
// missing/expired/revoked refresh tokens.
//
// The seeded user is NOT in any preconfigured groups.
func TestBrowserLoginWithSafeEmail(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	testBrowserLogin(
		c,
		s,
		jimmtest.HardcodedSafeUsername,
		jimmtest.HardcodedSafePassword,
		"user-jimm-test@canonical.com",
		"jimm-test",
		nil,
	)
}

func TestBrowserLoginWithUnsafeEmail(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	testBrowserLogin(
		c,
		s,
		jimmtest.HardcodedUnsafeUsername,
		jimmtest.HardcodedUnsafePassword,
		"user-jimm-test43cc8c@canonical.com",
		"jimm-test43cc8c",
		nil,
	)
}

// TestBrowserLoginWithSeededGroups tests that groups are properly propagated
// through the browser login flow and set in the session cookie.
func TestBrowserLoginWithSeededGroups(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	testBrowserLogin(
		c,
		s,
		jimmtest.HardcodedGroupUsername,
		jimmtest.HardcodedGroupPassword,
		"user-"+jimmtest.HardcodedGroupEmail,
		strings.Split(jimmtest.HardcodedGroupEmail, "@")[0],
		[]string{jimmtest.OIDCGroupsTestGroupName},
	)
}

func testBrowserLogin(c *qt.C, s jimmtest.JimmWithControllers, username, password, expectedEmail, expectedDisplayName string, expectedGroups []string) {
	// The setup runs a browser login with callback, ultimately retrieving
	// a logged in user by cookie.
	sqldb, err := s.JIMM.Database.DB.DB()
	c.Assert(err, qt.IsNil)

	sessionStore, err := pgstore.NewPGStoreFromPool(sqldb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)
	// Don't close the session store as it will close the underlying sql.DB which is shared with gorm and used by the JIMM instance.

	cookie, err := jimmtest.RunBrowserLogin(
		s.JIMM.Database,
		sessionStore,
		username,
		password,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(cookie, qt.Not(qt.Equals), "")

	cookies := jimmtest.ParseCookies(cookie)
	c.Assert(cookies, qt.HasLen, 1)

	if expectedGroups != nil {
		req, err := http.NewRequest("GET", "", nil)
		c.Assert(err, qt.IsNil)
		req.AddCookie(cookies[0])

		session, err := sessionStore.Get(req, auth.SessionName)
		c.Assert(err, qt.IsNil)
		c.Assert(session.Values[auth.SessionGroupsKey], qt.DeepEquals, expectedGroups)
	}

	jar, err := cookiejar.New(nil)
	c.Assert(err, qt.IsNil)

	// Now we move this cookie to the JIMM server on the admin suite and
	// set the cookie on the jimm test server url so that the cookie can be
	// sent on WS calls.
	jimmURL, err := url.Parse(s.Server.URL)
	c.Assert(err, qt.IsNil)
	jar.SetCookies(jimmURL, cookies)

	conn := s.OpenWithDialWebsocket(
		c,
		&api.Info{
			SkipLogin: true,
		},
		"test",
		getDialWebsocketWithCustomCookieJar(jar),
	)
	defer conn.Close()

	lr := &jujuparams.LoginResult{}
	err = conn.APICall("Admin", 4, "", "LoginWithSessionCookie", nil, lr)
	c.Assert(err, qt.IsNil)

	c.Assert(lr.UserInfo.Identity, qt.Equals, expectedEmail)
	c.Assert(lr.UserInfo.DisplayName, qt.Equals, expectedDisplayName)
}

// TestBrowserLoginNoCookie attempts to login without a cookie.
func TestBrowserLoginNoCookie(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	conn := s.Open(
		c,
		&api.Info{
			SkipLogin: true,
		},
		"test", nil)
	defer conn.Close()

	lr := &jujuparams.LoginResult{}
	err := conn.APICall("Admin", 4, "", "LoginWithSessionCookie", nil, lr)
	c.Assert(err, qt.ErrorMatches, `missing cookie identity \(unauthorized access\)`)
}

// TestDeviceLogin takes a test user through the flow of logging into jimm
// via the correct facades. All are done in a single test to see the flow end-2-end.
//
// Within the test are clear comments explaining what is happening when and why.
// Please refer to these comments for further details.
func TestDeviceLoginWithSeededGroups(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	conn := s.Open(c, &api.Info{
		SkipLogin: true,
	}, "test@canonical.com", nil)
	defer conn.Close()

	err := s.JIMM.Database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	// We create a http client to keep the same cookies across all requests
	// using a simple jar.
	jar, err := cookiejar.New(nil)
	c.Assert(err, qt.IsNil)

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
	c.Assert(err, qt.IsNil)
	c.Assert(loginDeviceResp.UserCode, qt.Not(qt.IsNil))
	c.Assert(loginDeviceResp.VerificationURI, qt.Equals, "http://localhost:8082/realms/jimm/device")

	// Step 2, complete the user side of the authentication by sending the
	// user code to the verification URI using the "complete" method.
	userResp, err := client.Get(loginDeviceResp.VerificationURI + "?user_code=" + loginDeviceResp.UserCode)
	c.Assert(err, qt.IsNil)
	body := userResp.Body
	defer body.Close()
	b, err := io.ReadAll(body)
	c.Assert(err, qt.IsNil)
	loginForm := string(b)

	// Step 2.1, handle the login form (see this func for more details)
	handleLoginForm(c, loginForm, client, jimmtest.HardcodedGroupUsername, jimmtest.HardcodedGroupPassword)

	// Step 3, after the user has entered the user code, the polling for an access
	// token will complete. The polling can begin before OR after the user has entered the
	// user code, for the simplicity of testing, we are retrieving it AFTER.
	var sessionTokenResp params.GetDeviceSessionTokenResponse
	err = conn.APICall("Admin", 4, "", "GetDeviceSessionToken", nil, &sessionTokenResp)
	c.Assert(err, qt.IsNil)
	// Ensure it is base64 and decodable
	decodedToken, err := base64.StdEncoding.DecodeString(sessionTokenResp.SessionToken)
	c.Assert(err, qt.IsNil)

	verifiedToken, err := s.JIMM.OAuthAuthenticator.VerifySessionToken(sessionTokenResp.SessionToken)
	c.Assert(err, qt.IsNil)

	groups, err := auth.SessionGroupsFromToken(verifiedToken)
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.DeepEquals, []string{jimmtest.OIDCGroupsTestGroupName})

	// Step 4, use this session token to "login".

	// Test no token present
	var loginResult jujuparams.LoginResult
	err = conn.APICall("Admin", 4, "", "LoginWithSessionToken", nil, &loginResult)
	c.Assert(err, qt.ErrorMatches, "no token presented.*")

	// Test token not base64 encoded
	err = conn.APICall("Admin", 4, "", "LoginWithSessionToken", params.LoginWithSessionTokenRequest{SessionToken: string(decodedToken)}, &loginResult)
	c.Assert(err, qt.ErrorMatches, "failed to decode token.*")

	// Test token base64 encoded passes authentication
	err = conn.APICall("Admin", 4, "", "LoginWithSessionToken", params.LoginWithSessionTokenRequest(sessionTokenResp), &loginResult)
	c.Assert(err, qt.IsNil)
	c.Assert(loginResult.UserInfo.Identity, qt.Equals, "user-"+jimmtest.HardcodedGroupEmail)
	c.Assert(loginResult.UserInfo.DisplayName, qt.Equals, strings.Split(jimmtest.HardcodedGroupEmail, "@")[0])

	// Finally, ensure db did indeed update the access token for this user
	updatedUser, err := dbmodel.NewIdentity(jimmtest.HardcodedGroupEmail)
	c.Assert(err, qt.IsNil)

	c.Assert(s.JIMM.Database.GetIdentity(context.Background(), updatedUser), qt.IsNil)
	// TODO(ale8k): Do we need to validate the token again for the test?
	// It has just been through a verifier etc and was returned directly
	// from the device grant?
	c.Assert(updatedUser.AccessToken, qt.Not(qt.Equals), "")

}

// handleLoginForm runs through the login process emulating the user typing in
// their username and password and then clicking consent, to complete
// the device login flow.
func handleLoginForm(c *qt.C, loginForm string, client *http.Client, username, password string) {
	// Step 2.2, now we'll be redirected to a sign-in page and must sign in.
	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(loginForm)
	c.Assert(match, qt.HasLen, 2)
	loginFormURL := match[1]

	// The username and password are hardcoded witih jimm-realm.json in our local
	// keycloak configuration for the jimm realm.
	v := url.Values{}
	v.Add("username", username)
	v.Add("password", password)
	loginResp, err := client.PostForm(loginFormURL, v)
	c.Assert(err, qt.IsNil)

	loginRespBody := loginResp.Body
	defer loginRespBody.Close()

	// Step 2.3, Keycloak may either show a consent form or complete directly.
	loginRespB, err := io.ReadAll(loginRespBody)
	c.Assert(err, qt.IsNil)
	loginRespS := string(loginRespB)

	re = regexp.MustCompile(`action="(.*?)" method=`)
	match = re.FindStringSubmatch(loginRespS)
	b := loginRespB
	if len(match) == 2 {
		consentFormURL := match[1]
		if !strings.HasPrefix(consentFormURL, "http://") && !strings.HasPrefix(consentFormURL, "https://") {
			consentFormURL = "http://localhost:8082" + consentFormURL
		}

		// We post the "yes" value to accept it.
		v = url.Values{}
		v.Add("accept", "Yes")
		consentResp, err := client.PostForm(consentFormURL, v)
		c.Assert(err, qt.IsNil)
		defer consentResp.Body.Close()

		// Read the response to ensure it is OK and has been accepted.
		b, err = io.ReadAll(consentResp.Body)
		c.Assert(err, qt.IsNil)
	}

	re = regexp.MustCompile(`Device Login Successful`)
	c.Assert(re.MatchString(string(b)), qt.Equals, true)
}

func TestLoginWithClientCredentials(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithRealAuthN())

	conn := s.Open(c, &api.Info{
		SkipLogin: true,
	}, "test@canonical.com", nil)
	defer conn.Close()

	const (
		// these are valid client credentials hardcoded into the jimm realm
		validClientID = "test-client-id"
		//nolint:gosec // Thinks credentials hardcoded.
		validClientSecret = "2M2blFbO4GX4zfggQpivQSxwWX1XGgNf"
	)

	var loginResult jujuparams.LoginResult
	err := conn.APICall("Admin", 4, "", "LoginWithClientCredentials", params.LoginWithClientCredentialsRequest{
		ClientID:     validClientID,
		ClientSecret: validClientSecret,
	}, &loginResult)
	c.Assert(err, qt.IsNil)
	c.Assert(loginResult.ControllerTag, qt.Equals, names.NewControllerTag(s.Params.ControllerUUID).String())
	c.Assert(loginResult.UserInfo.Identity, qt.Equals, names.NewUserTag("test-client-id@serviceaccount").String())

	err = conn.APICall("Admin", 4, "", "LoginWithClientCredentials", params.LoginWithClientCredentialsRequest{
		ClientID:     "invalid-client-id",
		ClientSecret: "invalid-secret",
	}, &loginResult)
	c.Assert(err, qt.ErrorMatches, `invalid client credentials: oauth2: "invalid_client" "Invalid client or Invalid client credentials" \(unauthorized access\)`)
}

// getDialWebsocketWithCustomCookieJar is mostly the default dialer configuration exception
// we need a dial websocket for juju containing a custom cookie jar to send cookies to
// a new server url when testing LoginWithSessionCookie. As such this closure simply
// passes the jar through.
func getDialWebsocketWithCustomCookieJar(jar *cookiejar.Jar) func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
	// Copied from github.com/juju/juju@v0.0.0-20240304110523-55fb5d03683b/api/apiclient.go
	dialWebsocket := func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
		url, err := url.Parse(urlStr)
		if err != nil {
			return nil, errors.Trace(err)
		}

		netDialer := net.Dialer{}
		dialer := &websocket.Dialer{
			NetDial: func(netw, addr string) (net.Conn, error) {
				if addr == url.Host {
					addr = ipAddr
				}
				return netDialer.DialContext(ctx, netw, addr)
			},
			Proxy:            proxy.DefaultConfig.GetProxy,
			HandshakeTimeout: 45 * time.Second,
			TLSClientConfig:  tlsConfig,
			// We update the jar so that the cookies retrieved from RunBrowserLogin
			// can be sent in the LoginWithSessionCookie call.
			Jar: jar,
		}

		c, resp, err := dialer.Dial(urlStr, nil)
		if err != nil {
			if err == websocket.ErrBadHandshake {
				defer resp.Body.Close()
				body, readErr := io.ReadAll(resp.Body)
				if readErr == nil {
					err = errors.Errorf(
						"%s (%s)",
						strings.TrimSpace(string(body)),
						http.StatusText(resp.StatusCode),
					)
				}
			}
			return nil, errors.Trace(err)
		}
		return jsoncodec.NewWebsocketConn(c), nil
	}
	return dialWebsocket
}
