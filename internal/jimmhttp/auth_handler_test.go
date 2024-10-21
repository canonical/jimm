// Copyright 2024 Canonical.
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
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func setupDbAndSessionStore(c *qt.C) (*db.Database, sessions.Store) {
	// Setup db ahead of time so we have access to session store
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	c.Assert(db.Migrate(context.Background(), false), qt.IsNil)

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

	pgSession := sessionStore.(*pgstore.PGStore)
	pgSession.Cleanup(time.Nanosecond)

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
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - no state cookie present")
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
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - state does not match")
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
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - missing auth code")
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
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusForbidden)+" - authorisation code exchange failed")
}
