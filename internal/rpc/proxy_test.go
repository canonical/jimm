// Copyright 2024 Canonical Ltd.

package rpc_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/oauth2"

	apiparams "github.com/canonical/jimm/v3/api/params"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpc"
)

type message struct {
	RequestID uint64                 `json:"request-id,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Version   int                    `json:"version,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Request   string                 `json:"request,omitempty"`
	Params    json.RawMessage        `json:"params,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ErrorCode string                 `json:"error-code,omitempty"`
	ErrorInfo map[string]interface{} `json:"error-info,omitempty"`
	Response  json.RawMessage        `json:"response,omitempty"`
}

func TestProxySocketsAdminFacade(t *testing.T) {
	c := qt.New(t)

	const (
		clientID     = "test-client-id"
		clientSecret = "test-client-secret"
	)

	loginData, err := json.Marshal(params.LoginRequest{
		AuthTag: names.NewUserTag("alice@wonderland.io").String(),
		Token:   "dGVzdCB0b2tlbg==",
	})
	c.Assert(err, qt.IsNil)

	serviceAccountLoginData, err := json.Marshal(params.LoginRequest{
		AuthTag: names.NewUserTag("test-client-id@serviceaccount").String(),
		Token:   "dGVzdCB0b2tlbg==",
	})
	c.Assert(err, qt.IsNil)

	ccData, err := json.Marshal(apiparams.LoginWithClientCredentialsRequest{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
	c.Assert(err, qt.IsNil)

	tests := []struct {
		about                     string
		messageToSend             message
		authenticateEntityID      string
		expectedClientResponse    *message
		expectedControllerMessage *message
		oauthAuthenticatorError   error
	}{{
		about: "login device call - client gets response with both user code and verification uri",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginDevice",
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Response:  []byte(`{"verification-uri":"http://no-such-uri.canonical.com","user-code":"test-user-code"}`),
		},
	}, {
		about: "login device call, but the authenticator returns an error",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginDevice",
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Error:     "a silly error",
		},
		oauthAuthenticatorError: errors.E("a silly error"),
	}, {
		about: "get device session token call - client gets response with a session token",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "GetDeviceSessionToken",
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Response:  []byte(`{"session-token":"test session token"}`),
		},
	}, {
		about: "get device session token call, but the authenticator returns an error",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "GetDeviceSessionToken",
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Error:     "a silly error",
		},
		oauthAuthenticatorError: errors.E("a silly error"),
	}, {
		about: "login with session token - a login message is sent to the controller",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginWithSessionToken",
			Params:    []byte(`{"client-id": "test session token"}`),
		},
		expectedControllerMessage: &message{
			RequestID: 1,
			Type:      "Admin",
			Version:   3,
			Request:   "Login",
			Params:    loginData,
		},
	}, {
		about: "login with session token, but authenticator returns an error",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginWithSessionToken",
			Params:    []byte(`{"client-id": "test session token"}`),
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Error:     "unauthorized access",
			ErrorCode: "unauthorized access",
		},
		oauthAuthenticatorError: errors.E(errors.CodeUnauthorized),
	}, {
		about: "login with client credentials - a login message is sent to the controller",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginWithClientCredentials",
			Params:    ccData,
		},
		expectedControllerMessage: &message{
			RequestID: 1,
			Type:      "Admin",
			Version:   3,
			Request:   "Login",
			Params:    serviceAccountLoginData,
		},
	}, {
		about: "login with client credentials, but authenticator returns an error",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginWithClientCredentials",
			Params:    ccData,
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Error:     "unauthorized access",
			ErrorCode: "unauthorized access",
		},
		oauthAuthenticatorError: errors.E(errors.CodeUnauthorized),
	}, {
		about: "any other message - gets forwarded directly to the controller",
		messageToSend: message{
			RequestID: 1,
			Type:      "Client",
			Version:   7,
			Request:   "AnyMethod",
			Params:    []byte(`{"key":"value"}`),
		},
		expectedControllerMessage: &message{
			RequestID: 1,
			Type:      "Client",
			Version:   7,
			Request:   "AnyMethod",
			Params:    []byte(`{"key":"value"}`),
		},
	}, {
		about: "login with session cookie - a login message is sent to the controller",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginWithSessionCookie",
			Params:    ccData,
		},
		authenticateEntityID: "alice@wonderland.io",
		expectedControllerMessage: &message{
			RequestID: 1,
			Type:      "Admin",
			Version:   3,
			Request:   "Login",
			Params:    loginData,
		},
	}, {
		about: "login with session cookie - but there was no identity id in the cookie",
		messageToSend: message{
			RequestID: 1,
			Type:      "Admin",
			Version:   4,
			Request:   "LoginWithSessionCookie",
			Params:    ccData,
		},
		expectedClientResponse: &message{
			RequestID: 1,
			Error:     "unauthorized access",
			ErrorCode: "unauthorized access",
		},
		oauthAuthenticatorError: errors.E(errors.CodeUnauthorized),
	}}

	for _, test := range tests {
		t.Run(test.about, func(t *testing.T) {
			ctx := context.Background()
			clientWebsocket := newMockWebsocketConnection(10)
			controllerWebsocket := newMockWebsocketConnection(10)
			authenticator := &mockOAuthAuthenticator{
				email:        "alice@wonderland.io",
				clientID:     clientID,
				clientSecret: clientSecret,
				err:          test.oauthAuthenticatorError,
			}

			helpers := rpc.ProxyHelpers{
				ConnClient: clientWebsocket,
				TokenGen:   &mockTokenGenerator{},
				ConnectController: func(ctx context.Context) (rpc.WebsocketConnectionWithMetadata, error) {
					return rpc.WebsocketConnectionWithMetadata{
						Conn:           controllerWebsocket,
						ModelName:      "test model",
						ControllerUUID: uuid.NewString(),
					}, nil
				},
				AuditLog: func(*dbmodel.AuditLogEntry) {},
				JIMM: &mockJIMM{
					authenticator: authenticator,
				},
				AuthenticatedIdentityID: test.authenticateEntityID,
			}
			go rpc.ProxySockets(ctx, helpers)

			data, err := json.Marshal(test.messageToSend)
			c.Assert(err, qt.IsNil)
			select {
			case clientWebsocket.read <- data:
			default:
				c.Fatal("failed to send message")
			}
			if test.expectedClientResponse != nil {
				select {
				case data := <-clientWebsocket.write:
					c.Assert(string(data), qt.JSONEquals, test.expectedClientResponse)
				case <-time.Tick(10 * time.Minute):
					c.Fatal("time out waiting for response")
				}
			}
			if test.expectedControllerMessage != nil {
				select {
				case data := <-controllerWebsocket.write:
					c.Assert(string(data), qt.JSONEquals, test.expectedControllerMessage)
				case <-time.Tick(10 * time.Minute):
					c.Fatal("time out waiting for response")
				}
			}
		})

	}
}

type mockOAuthAuthenticator struct {
	jimm.OAuthAuthenticator

	err error

	email        string
	clientID     string
	clientSecret string

	updatedEmail string
}

func (m *mockOAuthAuthenticator) Device(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &oauth2.DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "test-user-code",
		VerificationURI:         "http://no-such-uri.canonical.com",
		VerificationURIComplete: "http://no-such-uri.canonical.com",
		Expiry:                  time.Now().Add(time.Minute),
		Interval:                int64(time.Minute.Seconds()),
	}, nil
}

func (m *mockOAuthAuthenticator) DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &oauth2.Token{}, nil
}

func (m *mockOAuthAuthenticator) ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &oidc.IDToken{}, nil
}

func (m *mockOAuthAuthenticator) Email(idToken *oidc.IDToken) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.email != "" {
		return m.email, nil
	}
	return "", errors.E(errors.CodeNotFound)
}

func (m *mockOAuthAuthenticator) UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error {
	if m.err != nil {
		return m.err
	}
	m.updatedEmail = email
	return nil
}

func (m *mockOAuthAuthenticator) VerifyClientCredentials(ctx context.Context, clientID string, clientSecret string) error {
	if m.err != nil {
		return m.err
	}
	if clientID == m.clientID && clientSecret == m.clientSecret {
		return nil
	}
	return errors.E(errors.CodeUnauthorized)
}

func (m *mockOAuthAuthenticator) MintSessionToken(email string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return "test session token", nil
}

func (m *mockOAuthAuthenticator) VerifySessionToken(token string) (jwt.Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	t := jwt.New()
	t.Set(jwt.SubjectKey, m.email)
	return t, nil
}

type mockJIMM struct {
	authenticator *mockOAuthAuthenticator
}

func (j *mockJIMM) OAuthAuthenticationService() jimm.OAuthAuthenticator {
	return j.authenticator
}

func (j *mockJIMM) GetOpenFGAUserAndAuthorise(ctx context.Context, email string) (*openfga.User, error) {
	identity, err := dbmodel.NewIdentity(email)
	if err != nil {
		return nil, err
	}
	return openfga.NewUser(
		identity,
		nil,
	), nil
}

func (j *mockJIMM) GetCredentialStore() credentials.CredentialStore {
	return jimmtest.NewInMemoryCredentialStore()
}

func newMockWebsocketConnection(capacity int) *mockWebsocketConnection {
	return &mockWebsocketConnection{
		read:  make(chan []byte, capacity),
		write: make(chan []byte, capacity),
	}
}

type mockWebsocketConnection struct {
	read  chan []byte
	write chan []byte
}

func (w *mockWebsocketConnection) ReadJSON(v interface{}) error {
	data := <-w.read

	return json.Unmarshal(data, v)
}

func (w *mockWebsocketConnection) WriteJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.write <- data

	return nil
}

func (w *mockWebsocketConnection) Close() error {
	close(w.read)
	return nil
}

type mockTokenGenerator struct {
	mu sync.RWMutex

	mt names.ModelTag
	ct names.ControllerTag
	ut names.UserTag
}

func (m *mockTokenGenerator) MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ut = user.ResourceTag()
	return []byte("test token"), nil
}

func (m *mockTokenGenerator) MakeToken(ctx context.Context, permissionMap map[string]interface{}) ([]byte, error) {
	return []byte("test token"), nil
}

func (m *mockTokenGenerator) SetTags(mt names.ModelTag, ct names.ControllerTag) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mt = mt
	m.ct = ct
}

func (m *mockTokenGenerator) GetUser() names.UserTag {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.ut
}
