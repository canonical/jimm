// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	qt "github.com/frankban/quicktest"
	"github.com/gorilla/sessions"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func setupDbAndSessionStore(c *qt.C) (*db.Database, sessions.Store) {
	// Setup db ahead of time so we have access to session store
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	c.Assert(db.Migrate(context.Background()), qt.IsNil)

	sqlDb, err := db.DB.DB()
	c.Assert(err, qt.IsNil)

	store, err := pgstore.NewPGStoreFromPool(sqlDb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)

	return db, store
}

func createClientWithStateCookie(c *qt.C, s *httptest.Server) *http.Client {
	jar, err := cookiejar.New(nil)
	c.Assert(err, qt.IsNil)
	jimmURL, err := url.Parse(s.URL)
	c.Assert(err, qt.IsNil)
	stateCookie := http.Cookie{Name: auth.StateKey, Value: "123"}
	jar.SetCookies(jimmURL, []*http.Cookie{&stateCookie})
	return &http.Client{Jar: jar}
}

// TestBrowserLoginAndLogout goes through the flow of a browser logging in, simulating
// the cookie state and handling the callbacks are as expected. Additionally handling
// the final callback to the dashboard emulating an endpoint. See RunBrowserLogin
// where we create an additional handler to simulate the final callback to the dashboard
// from JIMM.
//
// Finally, it calls the logout using the cookie containing the identity we wish to logout.
func TestBrowserLoginAndLogout(t *testing.T) {
	c := qt.New(t)

	// Login
	db, sessionStore := setupDbAndSessionStore(c)

	cookie, jimmHTTPServer, err := jimmtest.RunBrowserLoginAndKeepServerRunning(
		db,
		sessionStore,
		jimmtest.HardcodedSafeUsername,
		jimmtest.HardcodedSafePassword,
	)
	c.Assert(err, qt.IsNil)
	defer jimmHTTPServer.Close()
	c.Assert(cookie, qt.Not(qt.Equals), "")

	// Run a whoami logged in
	req, err := http.NewRequest("GET", jimmHTTPServer.URL+jimmhttp.AuthResourceBasePath+jimmhttp.WhoAmIEndpoint, nil)
	c.Assert(err, qt.IsNil)
	parsedCookies := jimmtest.ParseCookies(cookie)
	c.Assert(parsedCookies, qt.HasLen, 1)
	req.AddCookie(parsedCookies[0])

	res, err := http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusOK)
	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.JSONEquals, &params.WhoamiResponse{
		DisplayName: "jimm-test",
		Email:       "jimm-test@canonical.com",
	})

	// Logout
	req, err = http.NewRequest("GET", jimmHTTPServer.URL+jimmhttp.AuthResourceBasePath+jimmhttp.LogOutEndpoint, nil)
	c.Assert(err, qt.IsNil)
	req.AddCookie(parsedCookies[0])

	res, err = http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusOK)

	// Run a whoami logged out
	req, err = http.NewRequest("GET", jimmHTTPServer.URL+jimmhttp.AuthResourceBasePath+jimmhttp.WhoAmIEndpoint, nil)
	c.Assert(err, qt.IsNil)
	parsedCookies = jimmtest.ParseCookies(cookie)
	c.Assert(parsedCookies, qt.HasLen, 1)
	req.AddCookie(parsedCookies[0])

	res, err = http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusForbidden)

	// Run a logout with no identity
	req, err = http.NewRequest("GET", jimmHTTPServer.URL+jimmhttp.AuthResourceBasePath+jimmhttp.LogOutEndpoint, nil)
	c.Assert(err, qt.IsNil)
	res, err = http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusForbidden)
}

func TestCallbackFailsNoState(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s, err := jimmtest.SetupTestDashboardCallbackHandler("<no dashboard needed for this test>", db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer s.Close()

	u, err := url.Parse(s.URL)
	c.Assert(err, qt.IsNil)
	u = u.JoinPath(jimmhttp.AuthResourceBasePath, jimmhttp.CallbackEndpoint)
	res, err := http.Get(u.String())
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - no state cookie present\n")
}

func TestCallbackFailsStateNoMatch(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s, err := jimmtest.SetupTestDashboardCallbackHandler("<no dashboard needed for this test>", db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer s.Close()

	client := createClientWithStateCookie(c, s)
	callbackURL := s.URL + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint
	res, err := client.Get(callbackURL + "?state=567")
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - state does not match\n")
}

func TestCallbackFailsNoCodePresent(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s, err := jimmtest.SetupTestDashboardCallbackHandler("<no dashboard needed for this test>", db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer s.Close()

	client := createClientWithStateCookie(c, s)

	callbackURL := s.URL + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint
	res, err := client.Get(callbackURL + "?state=123")
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - missing auth code\n")
}

func TestCallbackFailsExchange(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s, err := jimmtest.SetupTestDashboardCallbackHandler("<no dashboard needed for this test>", db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer s.Close()

	client := createClientWithStateCookie(c, s)
	callbackURL := s.URL + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint
	c.Assert(err, qt.IsNil)
	res, err := client.Get(callbackURL + "?code=idonotexist&state=123")
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+` - authorisation code exchange failed: oauth2: "invalid_grant" "Code not valid"`+"\n")
}

// stubLoginAuthenticator is a minimal BrowserOAuthAuthenticator that only
// supports the Login flow, allowing the state cookie path to be exercised
// without a real identity provider.
type stubLoginAuthenticator struct {
	jimmhttp.BrowserOAuthAuthenticator
}

func (stubLoginAuthenticator) AuthCodeURL() (string, string, error) {
	return "http://localhost/idp/auth?state=teststate", "teststate", nil
}

// TestLoginStateCookiePath verifies that the OAuth state cookie is scoped to a
// path that includes the external base path under which JIMM is hosted, so the
// browser sends the cookie back when the identity provider redirects to the
// callback endpoint.
func TestLoginStateCookiePath(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		name         string
		basePath     string
		expectedPath string
	}{{
		name:         "no base path",
		basePath:     "",
		expectedPath: jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint,
	}, {
		name:         "with base path",
		basePath:     "/jimm-jimm",
		expectedPath: "/jimm-jimm" + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint,
	}, {
		name:         "base path with trailing slash",
		basePath:     "/jimm-jimm/",
		expectedPath: "/jimm-jimm" + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint,
	}}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			h, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
				Authenticator:             stubLoginAuthenticator{},
				DashboardFinalRedirectURL: "http://localhost/dashboard",
				BasePath:                  test.basePath,
			})
			c.Assert(err, qt.IsNil)

			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", jimmhttp.AuthResourceBasePath+jimmhttp.LoginEndpoint, nil)
			h.Login(rr, req)

			res := rr.Result()
			defer res.Body.Close()

			var stateCookie *http.Cookie
			for _, cookie := range res.Cookies() {
				if cookie.Name == auth.StateKey {
					stateCookie = cookie
					break
				}
			}
			c.Assert(stateCookie, qt.Not(qt.IsNil))
			c.Check(stateCookie.Path, qt.Equals, test.expectedPath)
		})
	}
}
