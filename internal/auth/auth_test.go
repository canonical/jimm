// Copyright 2025 Canonical.

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
	"strings"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
	"github.com/gorilla/sessions"
	jujunames "github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/login"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

const (
	testGroupScope    = "group"
	testGroupClaimKey = "groups"
)

func setupTestAuthSvc(ctx context.Context, c *qt.C, expiry time.Duration) (*auth.AuthenticationService, *db.Database, sessions.Store, func()) {
	return setupTestAuthSvcWithGroupClaimKey(ctx, c, expiry, "")
}

func setupTestAuthSvcWithGroupClaimKey(ctx context.Context, c *qt.C, expiry time.Duration, groupClaimKey string) (*auth.AuthenticationService, *db.Database, sessions.Store, func()) {
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	c.Assert(db.Migrate(ctx), qt.IsNil)

	sqldb, err := db.DB.DB()
	c.Assert(err, qt.IsNil)

	sessionStore, err := pgstore.NewPGStoreFromPool(sqldb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)

	// #nosec G101 Fake test credentials and keys.
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:              "http://localhost:8082/realms/jimm",
		ClientID:               "jimm-device",
		ClientSecret:           "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:                 []string{oidc.ScopeOpenID, "profile", "email", "group"},
		GroupClaimKey:          groupClaimKey,
		SessionTokenExpiry:     expiry,
		RedirectURL:            "http://localhost:8080/auth/callback",
		Store:                  db,
		SessionStore:           sessionStore,
		SessionCookieMaxAge:    60,
		JWTSessionKey:          "secret-key",
		SecureCookies:          false,
		ClientCredentialScopes: []string{testGroupScope},
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

	authSvc, _, _, cleanup := setupTestAuthSvcWithGroupClaimKey(ctx, c, time.Hour, testGroupClaimKey)
	defer cleanup()

	url, state, err := authSvc.AuthCodeURL()
	c.Assert(err, qt.IsNil)
	c.Assert(
		url,
		qt.Matches,
		regexp.MustCompile(`http:\/\/localhost:8082\/realms\/jimm\/protocol\/openid-connect\/auth\?client_id=jimm-device&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fauth%2Fcallback&response_type=code&scope=openid\+profile\+email\+group&state=.*`),
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

	ctx := context.Background()

	authSvc, db, _, cleanup := setupTestAuthSvcWithGroupClaimKey(ctx, c, time.Hour, testGroupClaimKey)
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
	c.Assert(match, qt.HasLen, 2)
	loginFormURL := match[1]

	v := url.Values{}
	v.Add("username", jimmtest.HardcodedGroupUsername)
	v.Add("password", jimmtest.HardcodedGroupPassword)
	loginResp, err := client.PostForm(loginFormURL, v)
	c.Assert(err, qt.IsNil)
	defer loginResp.Body.Close()

	// Post consent when Keycloak presents a consent page.
	b, err = io.ReadAll(loginResp.Body)
	c.Assert(err, qt.IsNil)

	re = regexp.MustCompile(`action="(.*?)" method=`)
	match = re.FindStringSubmatch(string(b))
	if len(match) == 2 {
		consentFormURL := match[1]
		if !strings.HasPrefix(consentFormURL, "http://") && !strings.HasPrefix(consentFormURL, "https://") {
			consentFormURL = "http://localhost:8082" + consentFormURL
		}
		v = url.Values{}
		v.Add("accept", "Yes")
		consentResp, err := client.PostForm(consentFormURL, v)
		c.Assert(err, qt.IsNil)
		defer consentResp.Body.Close()

		// Read consent response when present.
		b, err = io.ReadAll(consentResp.Body)
		c.Assert(err, qt.IsNil)
	}

	re = regexp.MustCompile(`Device Login Successful`)
	c.Assert(re.MatchString(string(b)), qt.IsTrue)

	// Retrieve access token
	token, err := authSvc.DeviceAccessToken(ctx, res)
	c.Assert(err, qt.IsNil)
	c.Assert(token, qt.IsNotNil)

	claims, err := authSvc.VerifyAndExtractIdentityClaims(ctx, token)
	c.Assert(err, qt.IsNil)
	c.Assert(claims.Subject, qt.Equals, jimmtest.HardcodedGroupUserID)
	c.Assert(claims.Email, qt.Equals, jimmtest.HardcodedGroupEmail)
	c.Assert(claims.Groups, qt.DeepEquals, []string{jimmtest.OIDCGroupsTestGroupName})

	missingClaimAuthSvc, _, _, missingCleanup := setupTestAuthSvcWithGroupClaimKey(ctx, c, time.Hour, "missing-groups")
	defer missingCleanup()

	missingClaims, err := missingClaimAuthSvc.VerifyAndExtractIdentityClaims(ctx, token)
	c.Assert(err, qt.IsNil)
	c.Assert(missingClaims.Subject, qt.Equals, jimmtest.HardcodedGroupUserID)
	c.Assert(missingClaims.Email, qt.Equals, jimmtest.HardcodedGroupEmail)
	c.Assert(missingClaims.Groups, qt.IsNil)

	// Update the identity
	err = authSvc.UpdateIdentity(ctx, claims.Email, token)
	c.Assert(err, qt.IsNil)

	updatedUser, err := dbmodel.NewIdentity(jimmtest.HardcodedGroupEmail)
	c.Assert(err, qt.IsNil)
	c.Assert(db.GetIdentity(ctx, updatedUser), qt.IsNil)
	c.Assert(updatedUser.AccessToken, qt.Not(qt.Equals), "")
	c.Assert(updatedUser.RefreshToken, qt.Not(qt.Equals), "")
}

// TestSessionTokensWithoutGroups tests both the minting and validation of JIMM
// session tokens when no groups are present.
func TestSessionTokensWithoutGroups(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	token, err := authSvc.MintSessionTokenWithGroups("jimm-test@canonical.com", nil)
	c.Assert(err, qt.IsNil)
	c.Assert(len(token) > 0, qt.IsTrue)

	jwtToken, err := authSvc.VerifySessionToken(token)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "jimm-test@canonical.com")
}

func TestSessionTokensWithGroups(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	groups := []string{"devops", "platform"}
	token, err := authSvc.MintSessionTokenWithGroups("jimm-test@canonical.com", groups)
	c.Assert(err, qt.IsNil)

	jwtToken, err := authSvc.VerifySessionToken(token)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "jimm-test@canonical.com")

	groupsClaim, err := auth.SessionGroupsFromToken(jwtToken)
	c.Assert(err, qt.IsNil)
	c.Assert(groupsClaim, qt.DeepEquals, groups)
}

func TestSessionGroupsFromTokenRejectsInvalidClaimType(t *testing.T) {
	c := qt.New(t)

	token, err := jwt.NewBuilder().
		Subject("jimm-test@canonical.com").
		Claim(auth.SessionTokenGroupsClaimKey, "devops").
		Build()
	c.Assert(err, qt.IsNil)

	groups, err := auth.SessionGroupsFromToken(token)
	c.Assert(err, qt.ErrorMatches, `invalid "groups" claim type string`)
	c.Assert(groups, qt.IsNil)
}

func TestSessionTokenRejectsExpiredToken(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	noDuration := time.Duration(0)
	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, noDuration)
	defer cleanup()

	token, err := authSvc.MintSessionTokenWithGroups("jimm-test@canonical.com", nil)
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

	token, err := authSvc.MintSessionTokenWithGroups("", nil)
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

	groups, err := authSvc.VerifyClientCredentials(ctx, validClientID, validClientSecret)
	c.Assert(err, qt.IsNil)
	// The local Keycloak service-account token currently does not include the
	// groups claim, even when the group scope is requested.
	c.Assert(groups, qt.IsNil)

	_, err = authSvc.VerifyClientCredentials(ctx, "invalid-client-id", validClientSecret)
	c.Assert(err, qt.ErrorMatches, "invalid client credentials.*")
}

func TestVerifyClientCredentialsInGroups(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	const (
		validClientID = "test-client-id-with-groups"
		//nolint:gosec // Thinks hardcoded credentials.
		validClientSecret = "d6JVj6NDXYx56muG6ZmjWMLcJnIYQjD0"
	)

	authSvc, _, _, cleanup := setupTestAuthSvcWithGroupClaimKey(ctx, c, time.Hour, testGroupClaimKey)
	defer cleanup()

	groups, err := authSvc.VerifyClientCredentials(ctx, validClientID, validClientSecret)
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.DeepEquals, []string{jimmtest.OIDCGroupsTestGroupName})
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

func TestCreateBrowserSessionWithNoGroups(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, _, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	err = authSvc.CreateBrowserSessionWithGroups(ctx, rec, req, "jimm-test@canonical.com", nil)
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
	c.Assert(session.Values[auth.SessionGroupsKey], qt.IsNil)
}

func TestBrowserLoginStoresExtractedGroups(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	_, db, sessionStore, cleanup := setupTestAuthSvcWithGroupClaimKey(ctx, c, time.Hour, testGroupClaimKey)
	defer cleanup()

	cookie, err := jimmtest.RunBrowserLogin(
		db,
		sessionStore,
		jimmtest.HardcodedGroupUsername,
		jimmtest.HardcodedGroupPassword,
	)
	c.Assert(err, qt.IsNil)

	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	cookies := jimmtest.ParseCookies(cookie)
	req.AddCookie(cookies[0])

	session, err := sessionStore.Get(req, auth.SessionName)
	c.Assert(err, qt.IsNil)
	c.Assert(session.Values[auth.SessionIdentityKey], qt.Equals, jimmtest.HardcodedGroupEmail)
	c.Assert(session.Values[auth.SessionGroupsKey], qt.DeepEquals, []string{jimmtest.OIDCGroupsTestGroupName})
}

func TestKeycloakGroupUserAuthorisesViaIDPGroupContextualTuple(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, db, sessionStore, cleanup := setupTestAuthSvcWithGroupClaimKey(ctx, c, time.Hour, testGroupClaimKey)
	defer cleanup()

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	loginManager, err := login.NewLoginManager(db, ofgaClient, authSvc, jujunames.NewControllerTag("jimm"))
	c.Assert(err, qt.IsNil)

	cookie, err := jimmtest.RunBrowserLogin(
		db,
		sessionStore,
		jimmtest.HardcodedGroupUsername,
		jimmtest.HardcodedGroupPassword,
	)
	c.Assert(err, qt.IsNil)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)
	req.AddCookie(jimmtest.ParseCookies(cookie)[0])

	ctx, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.IsNil)

	user, err := loginManager.LoginWithSessionCookie(ctx, jimmtest.HardcodedGroupEmail)
	c.Assert(err, qt.IsNil)
	c.Assert(user.IDPGroupIDs, qt.DeepEquals, []string{jimmtest.OIDCGroupsTestGroupName})

	modelTag := jujunames.NewModelTag("00000002-0000-0000-0000-000000000001")
	err = ofgaClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewIdPGroupTag(jimmtest.OIDCGroupsTestGroupName), ofganames.MemberRelation),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(modelTag),
	})
	c.Assert(err, qt.IsNil)

	allowed, err := user.IsModelReader(ctx, modelTag)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsTrue)
}

func TestCreateBrowserSessionWithGroups(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, _, sessionStore, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)

	groups := []string{"devops", "platform"}
	err = authSvc.CreateBrowserSessionWithGroups(ctx, rec, req, "jimm-test@canonical.com", groups)
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
	c.Assert(session.Values[auth.SessionGroupsKey], qt.DeepEquals, groups)
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
	c.Assert(err, qt.ErrorMatches, "failed to retrieve session.*")

	// Failure case 2: Value isn't valid but is base64 decoded
	req, err = http.NewRequest("GET", "", nil)
	c.Assert(err, qt.IsNil)
	req.AddCookie(&http.Cookie{
		Name:  auth.SessionName,
		Value: base64.StdEncoding.EncodeToString([]byte("bad cookie, very naughty, bad bad cookie")),
	})

	rec = httptest.NewRecorder()

	_, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.ErrorMatches, "failed to retrieve session.*")
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

	// AuthenticateBrowserSession should fail to refresh the users session.
	_, err = authSvc.AuthenticateBrowserSession(ctx, rec, req)
	c.Assert(err, qt.ErrorMatches, ".*failed to refresh token: oauth2: token expired and refresh token is not set")
}

func TestNewMigrationToken(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	authSvc, _, _, cleanup := setupTestAuthSvc(ctx, c, time.Hour)
	defer cleanup()

	// Generate a migration token for a user
	migrationToken, err := authSvc.NewMigrationToken(ctx, "alice@canonical.com", []string{"team-a"})
	c.Assert(err, qt.IsNil)
	c.Assert(migrationToken, qt.Not(qt.Equals), "")

	jwtToken, err := authSvc.VerifySessionToken(migrationToken)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "alice@canonical.com")
	migrationGroups, err := auth.SessionGroupsFromToken(jwtToken)
	c.Assert(err, qt.IsNil)
	c.Assert(migrationGroups, qt.DeepEquals, []string{"team-a"})

	// Generate a migration token for a service account
	migrationToken, err = authSvc.NewMigrationToken(ctx, "cde78135-f1b1-436f-8461-58461fa95914@serviceaccount", nil)
	c.Assert(err, qt.IsNil)
	c.Assert(migrationToken, qt.Not(qt.Equals), "")

	jwtToken, err = authSvc.VerifySessionToken(migrationToken)
	c.Assert(err, qt.IsNil)
	c.Assert(jwtToken.Subject(), qt.Equals, "cde78135-f1b1-436f-8461-58461fa95914@serviceaccount")
}
