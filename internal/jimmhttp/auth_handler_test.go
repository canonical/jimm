package jimmhttp_test

import (
	"context"
	"fmt"
	"io"
	"net"
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

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimmhttp"
	"github.com/canonical/jimm/internal/jimmtest"
)

func setupDbAndSessionStore(c *qt.C) (*db.Database, *pgstore.PGStore) {
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

func setupTestServer(c *qt.C, dashboardURL string, db *db.Database, sessionStore *pgstore.PGStore) *httptest.Server {
	// Find a random free TCP port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, qt.IsNil)
	port := fmt.Sprintf("%d", listener.Addr().(*net.TCPAddr).Port)

	// Create unstarted server to enable auth service
	s := httptest.NewUnstartedServer(nil)
	s.Listener = listener

	// Remember redirect url to check it matches after test server starts
	redirectURL := "http://127.0.0.1:" + port + "/callback"
	authSvc, err := auth.NewAuthenticationService(context.Background(), auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		ClientID:           "jimm-device",
		ClientSecret:       "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:             []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry: time.Hour,
		// Now we know the port the test server is running on
		RedirectURL: redirectURL,
		Store:       db,
	})
	c.Assert(err, qt.IsNil)

	h, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
		Authenticator:             authSvc,
		DashboardFinalRedirectURL: dashboardURL,
		SessionStore:              sessionStore,
		SecureCookies:             false,
		CookieExpiry:              86400,
	})
	c.Assert(err, qt.IsNil)

	s.Config.Handler = h.Routes()

	s.Start()

	// Ensure redirectURL is matching port on listener
	c.Assert(s.URL+"/callback", qt.Equals, redirectURL)

	return s
}

// TestBrowserAuth goes through the flow of a browser logging in, simulating
// the cookie state and handling the callbacks are as expected. Additionally handling
// the final callback to the dashboard emulating an endpoint. See setupTestServer
// where we create an additional handler to simulate the final callback to the dashboard
// from JIMM.
func TestBrowserAuth(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)

	// Setup final test redirect url server, to emulate
	// the dashboard receiving the final piece of the flow
	dashboardResponse := "dashboard received final callback"
	dashboard := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, dashboardResponse)
				sessionCookie, _ := r.Cookie("jimm-browser-session")
				c.Assert(sessionCookie.Name, qt.Equals, "jimm-browser-session")
				c.Assert(sessionCookie.Value, qt.Not(qt.Equals), "")
				// Check the session exist in db
				session, err := sessionStore.Get(r, "jimm-browser-session")
				c.Assert(err, qt.IsNil)
				c.Assert(session.Values["jimm-session"], qt.Equals, "jimm-test@canonical.com")
			},
		),
	)
	defer dashboard.Close()

	s := setupTestServer(c, dashboard.URL, db, sessionStore)
	defer s.Close()

	jar, err := cookiejar.New(nil)
	c.Assert(err, qt.IsNil)

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Println("redirected to", req.URL)
			return nil
		},
	}

	res, err := client.Get(s.URL + "/login")
	c.Assert(err, qt.IsNil)
	c.Assert(res.StatusCode, qt.Equals, http.StatusOK)

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)

	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(string(b))
	loginFormUrl := match[1]

	v := url.Values{}
	v.Add("username", "jimm-test")
	v.Add("password", "password")
	loginResp, err := client.PostForm(loginFormUrl, v)
	c.Assert(err, qt.IsNil)

	b, err = io.ReadAll(loginResp.Body)
	c.Assert(err, qt.IsNil)

	c.Assert(string(b), qt.Equals, dashboardResponse)
	c.Assert(loginResp.StatusCode, qt.Equals, 200)

	defer loginResp.Body.Close()
}

func TestCallbackFailsNoCodePresent(t *testing.T) {
	c := qt.New(t)

	db, sessionStore := setupDbAndSessionStore(c)
	s := setupTestServer(c, "<no dashboard needed for this test>", db, sessionStore)
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
	s := setupTestServer(c, "<no dashboard needed for this test>", db, sessionStore)
	defer s.Close()

	// Test with no code present at all
	res, err := http.Get(s.URL + "/callback?code=idonotexist")
	c.Assert(err, qt.IsNil)

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, http.StatusText(http.StatusBadRequest))
}
