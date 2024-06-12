// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
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

const (
	// Note that these values are deliberately different to make sure we're not
	// reusing/misusing them.
	JWTTestSecret      = "test-secret"
	SessionStoreSecret = "another-test-secret"
)

// A SimpleTester is a simple version of the test interface
// that both the GoChecker and QuickTest checker satisfy.
// Useful for enabling test setup functions to fail without a panic.
type SimpleTester interface {
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

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
}

func NewMockOAuthAuthenticator(secretKey string) MockOAuthAuthenticator {
	return MockOAuthAuthenticator{}
}

// VerifySessionToken provides the mock implementation for verifying session tokens.
// Allowing JIMM tests to create their own session tokens that will always be accepted.
// Notice the use of jwt.ParseInsecure to skip JWT signature verification.
func (m MockOAuthAuthenticator) VerifySessionToken(token string) (jwt.Token, error) {
	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("authentication failed, failed to decode token")
	}

	parsedToken, err := jwt.ParseInsecure(decodedToken)
	if err != nil {
		return nil, err
	}

	if _, err = mail.ParseAddress(parsedToken.Subject()); err != nil {
		return nil, errors.New("failed to parse email")
	}
	return parsedToken, nil
}

func (m MockOAuthAuthenticator) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	return ctx, errors.New("authentication failed")
}

// NewUserSessionLogin returns a login provider than be used with Juju Dial Opts
// to define how login will take place. In this case we login using a session token
// that the JIMM server should verify with the same test secret.
func NewUserSessionLogin(c SimpleTester, username string) api.LoginProvider {
	email := convertUsernameToEmail(username)
	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(1 * time.Hour)).
		Build()
	if err != nil {
		c.Fatalf("failed to generate test session token")
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(JWTTestSecret)))
	if err != nil {
		c.Fatalf("failed to sign test session token")
	}

	b64Token := base64.StdEncoding.EncodeToString(freshToken)
	return api.NewSessionTokenLoginProvider(b64Token, nil, nil)
}

func convertUsernameToEmail(username string) string {
	if !strings.Contains(username, "@") {
		return username + "@canonical.com"
	}
	return username
}

func SetupTestDashboardCallbackHandler(browserURL string, db *db.Database, sessionStore sessions.Store) (*httptest.Server, error) {
	// Find a random free TCP port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)

	// Create unstarted server to enable auth service
	s := httptest.NewUnstartedServer(nil)
	s.Listener = listener

	// Remember redirect url to check it matches after test server starts
	redirectURL := "http://127.0.0.1:" + port + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint
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
		SessionSecretKey:    "test-secret",
	})
	if err != nil {
		return nil, err
	}

	h, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
		Authenticator:             authSvc,
		DashboardFinalRedirectURL: browserURL,
		SecureCookies:             false,
	})
	if err != nil {
		return nil, err
	}

	mux := chi.NewMux()
	mux.Mount(jimmhttp.AuthResourceBasePath, h.Routes())
	s.Config.Handler = mux

	s.Start()

	// Ensure redirectURL is matching port on listener
	callbackURL := s.URL + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint
	if callbackURL != redirectURL {
		return s, errors.New("server callback does not match redirectURL")
	}

	return s, nil
}

func RunBrowserLoginAndKeepServerRunning(db *db.Database, sessionStore sessions.Store, username, password string) (string, *httptest.Server, error) {
	cookieString, jimmHTTPServer, err := runBrowserLogin(db, sessionStore, username, password)
	return cookieString, jimmHTTPServer, err
}

func RunBrowserLogin(db *db.Database, sessionStore sessions.Store, username, password string) (string, error) {
	cookieString, jimmHTTPServer, err := runBrowserLogin(db, sessionStore, username, password)
	defer jimmHTTPServer.Close()
	return cookieString, err
}

func ParseCookies(cookies string) []*http.Cookie {
	header := http.Header{}
	header.Add("Cookie", cookies)
	request := http.Request{Header: header}
	return request.Cookies()
}

func runBrowserLogin(db *db.Database, sessionStore sessions.Store, username, password string) (string, *httptest.Server, error) {
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

	s, err := SetupTestDashboardCallbackHandler(browser.URL, db, sessionStore)
	if err != nil {
		return cookieString, s, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return cookieString, s, err
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Println("redirected to", req.URL)
			return nil
		},
	}

	loginURL := s.URL + jimmhttp.AuthResourceBasePath + jimmhttp.LoginEndpoint
	res, err := client.Get(loginURL)
	if err != nil {
		return cookieString, s, err
	}

	if res.StatusCode != http.StatusOK {
		return cookieString, s, errors.New("status code not ok")
	}

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return cookieString, s, err
	}

	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(string(b))
	loginFormUrl := match[1]

	v := url.Values{}
	v.Add("username", username)
	v.Add("password", password)
	loginResp, err := client.PostForm(loginFormUrl, v)
	if err != nil {
		return cookieString, s, err
	}

	b, err = io.ReadAll(loginResp.Body)
	if err != nil {
		return cookieString, s, err
	}

	if string(b) != dashboardResponse {
		return cookieString, s, errors.New("dashboard response not equal")
	}
	if loginResp.StatusCode != http.StatusOK {
		return cookieString, s, errors.New("status code not ok")
	}

	loginResp.Body.Close()
	return cookieString, s, nil
}
