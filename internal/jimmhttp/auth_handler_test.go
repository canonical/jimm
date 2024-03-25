package jimmhttp_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	qt "github.com/frankban/quicktest"
	"github.com/gorilla/sessions"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimmtest"
)

func setupDbAndSessionStore(c *qt.C) (*db.Database, sessions.Store) {
	// Setup db ahead of time so we have access to session store
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, func() time.Time { return time.Now() }),
	}
	c.Assert(db.Migrate(context.Background(), false), qt.IsNil)

	sqlDb, err := db.DB.DB()
	c.Assert(err, qt.IsNil)

	store, err := pgstore.NewPGStoreFromPool(sqlDb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)

	return db, store
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

	cookie, jimmHTTPServer, err := jimmtest.RunBrowserLoginAndKeepServerRunning(db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer jimmHTTPServer.Close()
	c.Assert(cookie, qt.Not(qt.Equals), "")

	// Run a whoami logged in
	req, err := http.NewRequest("GET", jimmHTTPServer.URL+"/whoami", nil)
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
	req, err = http.NewRequest("GET", jimmHTTPServer.URL+"/logout", nil)
	c.Assert(err, qt.IsNil)
	req.AddCookie(parsedCookies[0])

	res, err = http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusOK)

	// Run a whoami logged out
	req, err = http.NewRequest("GET", jimmHTTPServer.URL+"/whoami", nil)
	c.Assert(err, qt.IsNil)
	parsedCookies = jimmtest.ParseCookies(cookie)
	c.Assert(parsedCookies, qt.HasLen, 1)
	req.AddCookie(parsedCookies[0])

	res, err = http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusInternalServerError)
	b, err = io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	// TODO(ale8k): Really it isn't an internal server error here, the session is just
	// missing in our store, we should probably bring this error up and return a forbidden.
	c.Assert(string(b), qt.Equals, "Internal Server Error")

	// Run a logout with no identity
	req, err = http.NewRequest("GET", jimmHTTPServer.URL+"/logout", nil)
	c.Assert(err, qt.IsNil)
	res, err = http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer res.Body.Close()
	c.Assert(res.StatusCode, qt.Equals, http.StatusForbidden)
}

func TestCallbackFailsNoCodePresent(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s, err := jimmtest.SetupTestDashboardCallbackHandler("<no dashboard needed for this test>", db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer s.Close()

	// Test with no code present at all
	res, err := http.Get(s.URL + "/callback")
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusBadRequest))
}

func TestCallbackFailsExchange(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s, err := jimmtest.SetupTestDashboardCallbackHandler("<no dashboard needed for this test>", db, sessionStore)
	c.Assert(err, qt.IsNil)
	defer s.Close()

	// Test with no code present at all
	res, err := http.Get(s.URL + "/callback?code=idonotexist")
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusBadRequest))
}
