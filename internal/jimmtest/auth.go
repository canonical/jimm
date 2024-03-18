// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmhttp"
	"github.com/canonical/jimm/internal/openfga"
)

var (
	jwtTestSecret = "test-secret"
)

// An Authenticator is an implementation of jimm.Authenticator that returns
// the stored user and error.
type Authenticator struct {
	User *openfga.User
	Err  error
}

// Authenticate implements jimm.Authenticator.
func (a Authenticator) Authenticate(_ context.Context, _ *jujuparams.LoginRequest) (*openfga.User, error) {
	return a.User, a.Err
}

type MockOAuthAuthenticator struct {
	jimm.OAuthAuthenticator
	secretKey string
}

func NewMockOAuthAuthenticator(secretKey string) MockOAuthAuthenticator {
	return MockOAuthAuthenticator{secretKey: secretKey}
}

// VerifySessionToken provides the mock implementation for verifying session tokens.
// Allowing JIMM tests to create their own session tokens that will always be accepted.
func (m MockOAuthAuthenticator) VerifySessionToken(token string, secretKey string) (jwt.Token, error) {
	return auth.VerifySessionToken(token, m.secretKey)
}

func NewUserSessionLogin(username string) api.LoginProvider {
	email := convertUsernameToEmail(username)
	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(1 * time.Hour)).
		Build()
	if err != nil {
		panic("failed to generate test session token")
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(jwtTestSecret)))
	if err != nil {
		panic("failed to sign test session token")
	}

	b64Token := base64.StdEncoding.EncodeToString(freshToken)
	lp := api.NewSessionTokenLoginProvider(b64Token, nil, nil)
	return lp
}

func convertUsernameToEmail(username string) string {
	if !strings.Contains(username, "@") {
		return username + "@canonical.com"
	}
	return username
}

func SetupTestDashboardCallbackHandler(browserURL string, db *db.Database, sessionStore sessions.Store, cookieExpiry int) (*httptest.Server, error) {
	// Create unstarted server to enable auth service
	s := httptest.NewUnstartedServer(nil)
	// Setup random port listener
	minPort := 30000
	maxPort := 50000

	port := strconv.Itoa(rand.Intn(maxPort-minPort+1) + minPort)
	l, err := net.Listen("tcp", "localhost:"+port)
	if err != nil {
		return nil, err
	}

	// Set the listener with a random port
	s.Listener = l

	// Remember redirect url to check it matches after test server starts
	redirectURL := "http://127.0.0.1:" + port + "/callback"
	authSvc, err := auth.NewAuthenticationService(context.Background(), auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		ClientID:           "jimm-device",
		ClientSecret:       "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:             []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry: time.Hour,
		// Now we know the port the test server is running on
		RedirectURL:         redirectURL,
		Store:               db,
		SessionStore:        sessionStore,
		SessionCookieMaxAge: 60,
	})
	if err != nil {
		return nil, err
	}

	h, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
		Authenticator:             authSvc,
		DashboardFinalRedirectURL: browserURL,
		SecureCookies:             false,
		CookieExpiry:              cookieExpiry,
	})
	if err != nil {
		return nil, err
	}

	s.Config.Handler = h.Routes()

	s.Start()

	// Ensure redirectURL is matching port on listener
	if s.URL+"/callback" != redirectURL {
		return s, errors.New("server callback does not match redirectURL")
	}

	return s, nil
}

func RunBrowserLogin(db *db.Database, sessionStore sessions.Store, cookieExpiry int) (string, error) {
	var cookieString string

	// Setup final test redirect url server, to emulate
	// the dashboard receiving the final piece of the flow
	dashboardResponse := "dashboard received final callback"
	browser := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				cookieString = r.Header.Get("Cookie")
				w.Write([]byte(dashboardResponse))
			},
		),
	)
	defer browser.Close()

	s, err := SetupTestDashboardCallbackHandler(browser.URL, db, sessionStore, cookieExpiry)
	if err != nil {
		return cookieString, err
	}
	defer s.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return cookieString, err
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Println("redirected to", req.URL)
			return nil
		},
	}

	res, err := client.Get(s.URL + "/login")
	if err != nil {
		return cookieString, err
	}

	if res.StatusCode != http.StatusOK {
		return cookieString, errors.New("status code not ok")
	}

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return cookieString, err
	}

	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(string(b))
	loginFormUrl := match[1]

	v := url.Values{}
	v.Add("username", "jimm-test")
	v.Add("password", "password")
	loginResp, err := client.PostForm(loginFormUrl, v)
	if err != nil {
		return cookieString, err
	}

	b, err = io.ReadAll(loginResp.Body)
	if err != nil {
		return cookieString, err
	}

	if string(b) != dashboardResponse {
		return cookieString, errors.New("dashboard response not equal")
	}
	if loginResp.StatusCode != http.StatusOK {
		return cookieString, errors.New("status code not ok")
	}

	loginResp.Body.Close()
	return cookieString, nil
}

func ParseCookies(cookies string) []*http.Cookie {
	header := http.Header{}
	header.Add("Cookie", cookies)
	request := http.Request{Header: header}
	return request.Cookies()
}
