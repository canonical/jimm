// Copyright 2024 Canonical.

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
	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	jimmerrors "github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/openfga"
)

const (
	// Secrets used for auth in tests.
	//
	// Note that these values are deliberately different to make sure we're not reusing/misusing them.
	JWTTestSecret      = "jwt-test-secret"
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

type mockOAuthAuthenticator struct {
	jimm.OAuthAuthenticator
	c SimpleTester
	// PollingChan is used to simulate polling an OIDC server during the device flow.
	// It expects a username to be received that will be used to generate the user's access token.
	PollingChan     <-chan string
	polledUsername  string
	mockAccessToken string
}

// NewMockOAuthAuthenticator creates a mock authenticator for tests. An channel can be passed in
// when testing the device flow to simulate polling an OIDC server. Provide a nil channel
// if the device flow will not be used in the test.
func NewMockOAuthAuthenticator(c SimpleTester, testChan <-chan string) mockOAuthAuthenticator {
	return mockOAuthAuthenticator{c: c, PollingChan: testChan}
}

// Device is a mock implementation for the start of the device flow, returning dummy polling data.
func (m *mockOAuthAuthenticator) Device(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	return &oauth2.DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "test-user-code",
		VerificationURI:         "http://no-such-uri.canonical.com",
		VerificationURIComplete: "http://no-such-uri.canonical.com",
		Expiry:                  time.Now().Add(time.Minute),
		Interval:                int64(time.Minute.Seconds()),
	}, nil
}

// DeviceAccessToken is a mock implementation of the second step in the device flow where JIMM
// polls an OIDC server for the device code.
func (m *mockOAuthAuthenticator) DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	select {
	case username := <-m.PollingChan:
		m.polledUsername = username
		uuid, err := uuid.NewRandom()
		if err != nil {
			m.c.Fatalf("failed to generate UUID for device access token")
		}
		m.mockAccessToken = uuid.String()
	case <-ctx.Done():
		return &oauth2.Token{}, ctx.Err()
	}
	return &oauth2.Token{AccessToken: m.mockAccessToken}, nil
}

// VerifySessionToken provides the mock implementation for verifying session tokens.
// Allowing JIMM tests to create their own session tokens that will always be accepted.
// Notice the use of jwt.ParseInsecure to skip JWT signature verification.
func (m *mockOAuthAuthenticator) VerifySessionToken(token string) (jwt.Token, error) {
	errorFn := func(err error) error {
		return jimmerrors.E(err, jimmerrors.CodeSessionTokenInvalid)
	}
	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errorFn(errors.New("failed to decode token"))
	}

	parsedToken, err := jwt.ParseInsecure(decodedToken)
	if err != nil {
		return nil, errorFn(err)
	}

	if _, err = mail.ParseAddress(parsedToken.Subject()); err != nil {
		return nil, errorFn(errors.New("failed to parse email"))
	}
	return parsedToken, nil
}

// ExtractAndVerifyIDToken returns an ID token where the subject is equal to the username obtained during the device flow.
// The auth token must match the one returned during the device flow.
// If the polled username is empty it indicates an error that the device flow was not run prior to calling this function.
func (m *mockOAuthAuthenticator) ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error) {
	if m.polledUsername == "" {
		return &oidc.IDToken{}, errors.New("unknown user for mock auth login")
	}
	if m.mockAccessToken != oauth2Token.AccessToken {
		return &oidc.IDToken{}, errors.New("access token does not match the generated access token")
	}
	return &oidc.IDToken{Subject: m.polledUsername}, nil
}

// Email returns the subject from an ID token.
func (m *mockOAuthAuthenticator) Email(idToken *oidc.IDToken) (string, error) {
	return idToken.Subject, nil
}

// UpdateIdentity is a no-op mock.
func (m *mockOAuthAuthenticator) UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error {
	return nil
}

// MintSessionToken creates an unsigned session token with the email provided.
func (m *mockOAuthAuthenticator) MintSessionToken(email string) (string, error) {
	return newSessionToken(m.c, email, ""), nil
}

// AuthenticateBrowserSession unless overridden by the `AuthenticateBrowserSession_` field, it will return an authentication failure error.
func (m *mockOAuthAuthenticator) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	return ctx, errors.New("authentication failed")
}

// VerifyClientCredentials always returns a nil error.
func (m *mockOAuthAuthenticator) VerifyClientCredentials(ctx context.Context, clientID string, clientSecret string) error {
	return nil
}

// newSessionToken returns a serialised JWT that can be used in tests.
// Tests using a mock authenticator can provide an empty signatureSecret
// while integration tests must provide the same secret used when verifying JWTs.
func newSessionToken(c SimpleTester, username string, signatureSecret string) string {
	email := ConvertUsernameToEmail(username)
	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(1 * time.Hour)).
		Build()
	if err != nil {
		c.Fatalf("failed to generate test session token")
	}
	var serialisedToken []byte
	if signatureSecret != "" {
		serialisedToken, err = jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(JWTTestSecret)))
	} else {
		serialisedToken, err = jwt.NewSerializer().Serialize(token)
	}
	if err != nil {
		c.Fatalf("failed to sign/serialise token")
	}
	return base64.StdEncoding.EncodeToString(serialisedToken)
}

// NewUserSessionLogin returns a login provider than be used with Juju Dial Opts
// to define how login will take place. In this case we login using a session token
// that the JIMM server should verify with the same test secret.
func NewUserSessionLogin(c SimpleTester, username string) api.LoginProvider {
	b64Token := newSessionToken(c, username, JWTTestSecret)
	return api.NewSessionTokenLoginProvider(b64Token, nil, nil)
}

// ConvertUsernameToEmail appends an "@canonical.com" domain to a string if it doesn't already contain a domain.
func ConvertUsernameToEmail(username string) string {
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
		JWTSessionKey:       "test-secret",
		SecureCookies:       false,
	})
	if err != nil {
		return nil, err
	}

	h, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
		Authenticator:             authSvc,
		DashboardFinalRedirectURL: browserURL,
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
				if _, err := w.Write([]byte(dashboardResponse)); err != nil {
					zapctx.Error(context.Background(), "failed to write dashboard response", zap.Error(err))
				}

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
