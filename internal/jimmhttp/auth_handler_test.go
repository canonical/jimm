package jimmhttp_test

import (
	"context"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/jimmhttp"

	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
)

func setupTestServer(c *qt.C) *httptest.Server {
	// Create unstarted server to enable auth service
	s := httptest.NewUnstartedServer(nil)
	// Setup random port listener
	minPort := 30000
	maxPort := 50000

	port := strconv.Itoa(rand.Intn(maxPort-minPort+1) + minPort)
	l, err := net.Listen("tcp", "localhost:"+port)
	c.Assert(err, qt.IsNil)
	// Set the listener with a random port
	s.Listener = l

	// Remember redirect url to check it matches after test server starts
	redirectURL := "http://127.0.0.1:" + port + "/auth/callback"

	authSvc, err := auth.NewAuthenticationService(context.Background(), auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		ClientID:           "jimm-device",
		ClientSecret:       "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:             []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry: time.Hour,
		// Now we know the port the test server is running on
		RedirectURL: redirectURL,
	})
	c.Assert(err, qt.IsNil)

	r := jimmhttp.NewOAuthHandler(authSvc).Routes()
	s.Config.Handler = r

	s.Start()

	// Ensure redirectURL is matching port on listener
	c.Assert(s.URL+"/auth/callback", qt.Equals, redirectURL)

	return s
}

// Login is simply expected to redirect the user
func TestAuthLogin(t *testing.T) {
	c := qt.New(t)

	s := setupTestServer(c)
	defer s.Close()

	res, err := http.Get(s.URL + "/login")
	c.Assert(err, qt.IsNil)
	c.Assert(res.StatusCode, qt.Equals, http.StatusOK)
}
