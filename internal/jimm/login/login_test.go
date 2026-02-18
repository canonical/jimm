// Copyright 2025 Canonical.

package login_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/login"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

type loginManagerSuite struct {
	manager        *login.LoginManager
	user           *openfga.User
	db             *db.Database
	ofgaClient     *openfga.OFGAClient
	jimmTag        names.ControllerTag
	deviceFlowChan chan string
}

func (s *loginManagerSuite) Init(c *qt.C) {
	// Setup DB
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.deviceFlowChan = make(chan string, 1)

	s.jimmTag = names.NewControllerTag("foo")

	ctrl := gomock.NewController(c)
	mockAuthenticator := NewMockOAuthAuthenticator(ctrl)
	c.Cleanup(ctrl.Finish)

	mockAuthenticator.EXPECT().Device(gomock.Any()).Return(&oauth2.DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "test-user-code",
		VerificationURI:         "http://no-such-uri.canonical.com",
		VerificationURIComplete: "http://no-such-uri.canonical.com",
		Interval:                int64(time.Minute.Seconds()),
	}, nil).AnyTimes()

	mockAuthenticator.EXPECT().DeviceAccessToken(gomock.Any(), gomock.Any()).Return(&oauth2.Token{}, nil).AnyTimes()
	mockAuthenticator.EXPECT().ExtractAndVerifyIDToken(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockAuthenticator.EXPECT().Email(gomock.Any()).DoAndReturn(func(_ *oidc.IDToken) (string, error) {
		emailPrefix := "user-foo"
		select {
		case candidate := <-s.deviceFlowChan:
			emailPrefix = candidate
		default:
		}
		return fmt.Sprintf("%s@canonical.com", emailPrefix), nil
	}).AnyTimes()
	mockAuthenticator.EXPECT().UpdateIdentity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAuthenticator.EXPECT().MintSessionToken(gomock.Any()).DoAndReturn(func(email string) (string, error) {
		token, err := jwt.NewBuilder().
			Subject(email).
			Build()
		if err != nil {
			return "", err
		}
		serializedToken, err := jwt.NewSerializer().Serialize(token)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(serializedToken), nil
	}).AnyTimes()

	mockAuthenticator.EXPECT().VerifyClientCredentials(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, clientID string, clientSecret string) error {
		if clientID == "my-svc-acc" && clientSecret == "foo-secret" {
			return nil
		}
		return fmt.Errorf("invalid client credentials")
	}).AnyTimes()

	mockAuthenticator.EXPECT().VerifySessionToken(gomock.Any()).DoAndReturn(func(token string) (jwt.Token, error) {
		decodedToken, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return nil, errors.E(errors.CodeSessionTokenInvalid, "failed to decode token")
		}
		parsedToken, err := jwt.ParseInsecure(decodedToken)
		if err != nil {
			return nil, errors.E(errors.CodeSessionTokenInvalid, "failed to parse token")
		}
		return parsedToken, nil
	}).AnyTimes()

	s.manager, err = login.NewLoginManager(db, ofgaClient, mockAuthenticator, s.jimmTag)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i, ofgaClient)
}

func (s *loginManagerSuite) TestLoginDevice(c *qt.C) {
	c.Parallel()
	resp, err := s.manager.LoginDevice(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(*resp, qt.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), oauth2.DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "test-user-code",
		VerificationURI:         "http://no-such-uri.canonical.com",
		VerificationURIComplete: "http://no-such-uri.canonical.com",
		Interval:                int64(time.Minute.Seconds()),
	})
}

func (s *loginManagerSuite) TestGetDeviceSessionToken(c *qt.C) {
	c.Parallel()

	s.deviceFlowChan <- "user-foo"
	token, err := s.manager.GetDeviceSessionToken(context.Background(), nil)
	c.Assert(err, qt.IsNil)
	c.Assert(token, qt.Not(qt.Equals), "")

	decodedToken, err := base64.StdEncoding.DecodeString(token)
	c.Assert(err, qt.IsNil)

	parsedToken, err := jwt.ParseInsecure([]byte(decodedToken))
	c.Assert(err, qt.IsNil)
	c.Assert(parsedToken.Subject(), qt.Equals, "user-foo@canonical.com")
}

func (s *loginManagerSuite) TestLoginClientCredentials(c *qt.C) {
	c.Parallel()
	ctx := context.Background()
	invalidClientID := "123@123@"
	_, err := s.manager.LoginClientCredentials(ctx, invalidClientID, "foo-secret")
	c.Assert(err, qt.ErrorMatches, "invalid client ID")

	validClientID := "my-svc-acc"
	user, err := s.manager.LoginClientCredentials(ctx, validClientID, "foo-secret")
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "my-svc-acc@serviceaccount")
}

func (s *loginManagerSuite) TestLoginWithSessionToken(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	token, err := jwt.NewBuilder().
		Subject("alice@canonical.com").
		Build()
	c.Assert(err, qt.IsNil)
	serialisedToken, err := jwt.NewSerializer().Serialize(token)
	c.Assert(err, qt.IsNil)
	b64Token := base64.StdEncoding.EncodeToString(serialisedToken)

	_, err = s.manager.LoginWithSessionToken(ctx, "invalid-token")
	c.Assert(err, qt.ErrorMatches, "failed to decode token")

	user, err := s.manager.LoginWithSessionToken(ctx, b64Token)
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "alice@canonical.com")
}

func (s *loginManagerSuite) TestLoginWithSessionCookie(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	_, err := s.manager.LoginWithSessionCookie(ctx, "")
	c.Assert(err, qt.ErrorMatches, "missing cookie identity")

	user, err := s.manager.LoginWithSessionCookie(ctx, "alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "alice@canonical.com")
}

func (s *loginManagerSuite) TestGetOrCreateIdentity(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	ofgaUser, err := s.manager.GetOrCreateIdentity(ctx, "bob@canonical.com")
	c.Assert(err, qt.IsNil)
	// Username -> email
	c.Assert(ofgaUser.Name, qt.Equals, "bob@canonical.com")
	// As no display name was set for this user as they're being created this time over
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	// This user SHOULD NOT be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsFalse)

	// Next we'll update this user to an admin of JIMM and run the same tests.
	c.Assert(
		ofgaUser.SetControllerAccess(
			context.Background(),
			s.jimmTag,
			ofganames.AdministratorRelation,
		),
		qt.IsNil,
	)

	ofgaUser, err = s.manager.GetOrCreateIdentity(ctx, "bob@canonical.com")
	c.Assert(err, qt.IsNil)

	c.Assert(ofgaUser.Name, qt.Equals, "bob@canonical.com")
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	// This user SHOULD be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsTrue)
}

func (s *loginManagerSuite) TestUpdateLastLogin(c *qt.C) {
	c.Parallel()

	ctx := context.Background()

	ofgaUser, err := s.manager.UserLogin(ctx, "bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(ofgaUser, qt.Not(qt.IsNil))

	user := dbmodel.Identity{Name: "bob@canonical.com"}
	err = s.db.GetIdentity(ctx, &user)
	c.Assert(err, qt.IsNil)
	c.Assert(user.DisplayName, qt.Equals, "bob")
	c.Assert(user.LastLogin.Time, qt.Not(qt.Equals), time.Time{})
	c.Assert(user.LastLogin.Valid, qt.IsTrue)
}

func TestLoginManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &loginManagerSuite{})
}
