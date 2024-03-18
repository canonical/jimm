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

// TestBrowserAuth goes through the flow of a browser logging in, simulating
// the cookie state and handling the callbacks are as expected. Additionally handling
// the final callback to the dashboard emulating an endpoint. See setupTestServer
// where we create an additional handler to simulate the final callback to the dashboard
// from JIMM.
func TestBrowserAuth(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	cookie, err := jimmtest.RunBrowserLogin(db, sessionStore)
	c.Assert(err, qt.IsNil)
	c.Assert(cookie, qt.Not(qt.Equals), "")
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
