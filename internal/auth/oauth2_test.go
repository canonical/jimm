// Copyright 2024 Canonical.

package auth_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
	"github.com/gorilla/sessions"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func setupTestAuthSvc(ctx context.Context, c *qt.C, expiry time.Duration) (*auth.AuthenticationService, *db.Database, sessions.Store, func()) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	c.Assert(db.Migrate(ctx, false), qt.IsNil)

	sqldb, err := db.DB.DB()
	c.Assert(err, qt.IsNil)

	sessionStore, err := pgstore.NewPGStoreFromPool(sqldb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:           "http://localhost:8082/realms/jimm",
		ClientID:            "jimm-device",
		ClientSecret:        "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:              []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry:  expiry,
		RedirectURL:         "http://localhost:8080/auth/callback",
		Store:               db,
		SessionStore:        sessionStore,
		SessionCookieMaxAge: 60,
		JWTSessionKey:       "secret-key",
		SecureCookies:       false,
	})
	c.Assert(err, qt.IsNil)
	cleanup := func() {
		db.Close()
		sessionStore.Close()
	}
	return authSvc, db, sessionStore, cleanup
}

// This test requires the local docker compose to be running and keycloak
// to be available.
func TestAuthCodeURL(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	url, state, err := authSvc.AuthCodeURL()
	c.Assert(err, qt.IsNil)
	c.Assert(
		url,
		qt.Matches,
		regexp.MustCompile(`http:\/\/localhost:8082\/realms\/jimm\/protocol\/openid-connect\/auth\?client_id=jimm-device&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fauth%2Fcallback&response_type=code&scope=openid\+profile\+email&state=.*`),
	)
	c.Assert(len(state), qt.Not(qt.Equals), 0)
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

	authSvc, db, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

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

	// Update the identity
	err = authSvc.UpdateIdentity(ctx, email, token)
	c.Assert(err, qt.IsNil)

	updatedUser, err := dbmodel.NewIdentity(u.Email)
	c.Assert(err, qt.IsNil)
	c.Assert(db.GetIdentity(ctx, updatedUser), qt.IsNil)
	c.Assert(updatedUser.AccessToken, qt.Not(qt.Equals), "")
	c.Assert(updatedUser.RefreshToken, qt.Not(qt.Equals), "")
}

// TestSessionTokens tests both the minting and validation of JIMM
// session tokens.
func TestSessionTokens(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	token, err := authSvc.MintSessionToken("jimm-test@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	jwtToken, err := authSvc.VerifySessionToken(token)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "jimm-test@canonical.com")
}

func TestSessionTokenRejectsExpiredToken(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	noDuration := time.Duration(0)
	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, noDuration)
	defer cleanup()

	token, err := authSvc.MintSessionToken("jimm-test@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifySessionToken(token)
	c.Assert(err, qt.ErrorMatches, `JIMM session token expired`)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeSessionTokenInvalid)
}

func TestSessionTokenRejectsEmptyToken(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	noDuration := time.Duration(0)
	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, noDuration)
	defer cleanup()

	_, err := authSvc.VerifySessionToken("")
	c.Assert(err, qt.ErrorMatches, `no token presented`)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeSessionTokenInvalid)
}

func TestSessionTokenValidatesEmail(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	token, err := authSvc.MintSessionToken("")
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	_, err = authSvc.VerifySessionToken(token)
	c.Assert(err, qt.ErrorMatches, "failed to parse email")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeSessionTokenInvalid)
}

func TestVerifyClientCredentials(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	const (
		// these are valid client credentials hardcoded into the jimm realm
		validClientID = "test-client-id"
		//nolint:gosec // Thinks hardcoded credentials.
		validClientSecret = "2M2blFbO4GX4zfggQpivQSxwWX1XGgNf"
	)

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	err := authSvc.VerifyClientCredentials(ctx, validClientID, validClientSecret)
	c.Assert(err, qt.IsNil)

	err = authSvc.VerifyClientCredentials(ctx, "invalid-client-id", validClientSecret)
	c.Assert(err, qt.ErrorMatches, "invalid client credentials")
}

func assertSetCookiesIsCorrect(c *qt.C, parsedCookies []*http.Cookie) {
	assertHasCookie := func(name string, cookies []*http.Cookie) {
		found := false
		for _, v := range cookies {
			if v.Name == name {
				found = true
				break
			}
		}
		c.Assert(found, qt.IsTrue, qt.Commentf("cookie data assertion failed"))
	}
	assertHasCookie(auth.SessionName, parsedCookies)
	assertHasCookie("Path", parsedCookies)
	assertHasCookie("Expires", parsedCookies)
	assertHasCookie("Max-Age", parsedCookies)
}

func TestCreateBrowserSession(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, _, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	err = authSvc.CreateBrowserSession(ctx, rec, req, "jimm-test@canonical.com")
	c.Assert(err, qt.IsNil)

	cookies := rec.Header().Get("Set-Cookie")
	parsedCookies := jimmtest.ParseCookies(cookies)
	assertSetCookiesIsCorrect(c, parsedCookies)

	req.AddCookie(&http.Cookie{
		Name:  auth.SessionName,
		Value: parsedCookies[0].Value,
	})

	session, err := sessionStore.Get(req, auth.SessionName)
	c.Assert(err, qt.IsNil)
	c.Assert(session.Values[auth.SessionIdentityKey], qt.Equals, "jimm-test@canonical.com")
}

func TestAuthenticateBrowserSessionAndLogout(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, db, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	cookie, err := jimmtest.RunBrowserLogin(
		db,
		sessionStore,
		jimmtest.HardcodedSafeUsername,
		jimmtest.HardcodedSafePassword,
	)
	c.Assert(err, qt.IsNil)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	cookies := jimmtest.ParseCookies(cookie)

	req.AddCookie(cookies[0])

	ctx, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.IsNil)

	// Test whoami
	whoamiResp, err := authSvc.Whoami(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(whoamiResp.DisplayName, qt.Equals, "jimm-test")
	c.Assert(whoamiResp.Email, qt.Equals, "jimm-test@canonical.com")

	// Assert Set-Cookie present
	setCookieCookies := rec.Header().Get("Set-Cookie")
	parsedCookies := jimmtest.ParseCookies(setCookieCookies)
	assertSetCookiesIsCorrect(c, parsedCookies)

	// Test logout does indeed remove the cookie for us
	err = authSvc.Logout(ctx, rec, req)
	c.Assert(err, qt.IsNil)

	// Test whoami with empty context (simulating a logged out user)
	_, err = authSvc.Whoami(context.Background())
	c.Assert(err, qt.ErrorMatches, "no identity in context")

}

func TestAuthenticateBrowserSessionRejectsNoneDecryptableOrDecodableCookies(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, db, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	_, err := jimmtest.RunBrowserLogin(
		db,
		sessionStore,
		jimmtest.HardcodedSafeUsername,
		jimmtest.HardcodedSafePassword,
	)
	c.Assert(err, qt.IsNil)

	// Failure case 1: Bad base64 decoding
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)
	req.AddCookie(&http.Cookie{
		Name:  auth.SessionName,
		Value: "bad cookie, very naughty, bad bad cookie",
	})

	rec := httptest.NewRecorder()

	// The underlying error is a failed base64 decode
	_, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.ErrorMatches, "failed to retrieve session")

	// Failure case 2: Value isn't valid but is base64 decoded
	req, err = http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)
	req.AddCookie(&http.Cookie{
		Name:  auth.SessionName,
		Value: base64.StdEncoding.EncodeToString([]byte("bad cookie, very naughty, bad bad cookie")),
	})

	rec = httptest.NewRecorder()

	_, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.ErrorMatches, "failed to retrieve session")
}

func TestAuthenticateBrowserSessionHandlesExpiredAccessTokens(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, db, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	cookie, err := jimmtest.RunBrowserLogin(
		db,
		sessionStore,
		jimmtest.HardcodedSafeUsername,
		jimmtest.HardcodedSafePassword,
	)
	c.Assert(err, qt.IsNil)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	cookies := jimmtest.ParseCookies(cookie)

	req.AddCookie(cookies[0])

	// User exists from run browser login, but we're gonna
	// artificially expire their access token
	u, err := dbmodel.NewIdentity("jimm-test@canonical.com")
	c.Assert(err, qt.IsNil)
	err = db.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	previousToken := u.AccessToken

	u.AccessTokenExpiry = time.Now()
	err = db.UpdateIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	ctx, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.IsNil)

	// Check identity added
	identityId := auth.SessionIdentityFromContext(ctx)
	c.Assert(identityId, qt.Equals, "jimm-test@canonical.com")

	// Get identity again with new access token expiry and access token
	err = db.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	// Assert new access token is valid for at least 4 minutes(our setup is 5 minutes)
	c.Assert(u.AccessTokenExpiry.After(time.Now().Add(time.Minute*4)), qt.IsTrue)
	// Assert its not the same token as previous token
	c.Assert(u.AccessToken, qt.Not(qt.Equals), previousToken)
	// Assert Set-Cookie present
	setCookieCookies := rec.Header().Get("Set-Cookie")
	parsedCookies := jimmtest.ParseCookies(setCookieCookies)
	assertSetCookiesIsCorrect(c, parsedCookies)
}

func TestAuthenticateBrowserSessionHandlesMissingOrExpiredRefreshTokens(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, db, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	cookie, err := jimmtest.RunBrowserLogin(
		db,
		sessionStore,
		jimmtest.HardcodedSafeUsername,
		jimmtest.HardcodedSafePassword,
	)
	c.Assert(err, qt.IsNil)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	cookies := jimmtest.ParseCookies(cookie)

	req.AddCookie(cookies[0])

	// User exists from run browser login, but we're gonna
	// artificially expire their access token
	u, err := dbmodel.NewIdentity("jimm-test@canonical.com")
	c.Assert(err, qt.IsNil)
	err = db.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	// As our access token has "expired"
	u.AccessTokenExpiry = time.Now()
	// And we're missing a refresh token (the same case would apply for an expired refresh token
	// or any scenario where the token source cannot refresh the access token)
	u.RefreshToken = ""
	err = db.UpdateIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	// AuthenticateBrowserSession should fail to refresh the users session and delete
	// the current session, giving us the same cookie back with a max-age of -1.
	_, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.ErrorMatches, ".*failed to refresh token.*")

	// Assert that the header to delete the session is set correctly based
	// on a failed access token refresh due to refresh token issues.
	setCookieCookies := rec.Header().Get("Set-Cookie")
	c.Assert(
		setCookieCookies,
		qt.Equals,
		"jimm-browser-session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0; HttpOnly; SameSite=None",
	)
}
