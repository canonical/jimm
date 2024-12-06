// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestLoginDevice(t *testing.T) {
	c := qt.New(t)

	j := jimmtest.NewJIMM(c, nil)

	resp, err := j.LoginDevice(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(*resp, qt.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), oauth2.DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "test-user-code",
		VerificationURI:         "http://no-such-uri.canonical.com",
		VerificationURIComplete: "http://no-such-uri.canonical.com",
		Interval:                int64(time.Minute.Seconds()),
	})
}

func TestGetDeviceSessionToken(t *testing.T) {
	c := qt.New(t)
	pollingChan := make(chan string, 1)

	mockAuthenticator := jimmtest.NewMockOAuthAuthenticator(c, pollingChan)

	j := jimmtest.NewJIMM(c, &jimm.Parameters{
		OAuthAuthenticator: &mockAuthenticator,
	})

	pollingChan <- "user-foo"
	token, err := j.GetDeviceSessionToken(context.Background(), nil)
	c.Assert(err, qt.IsNil)
	c.Assert(token, qt.Not(qt.Equals), "")
	decodedToken, err := base64.StdEncoding.DecodeString(token)
	c.Assert(err, qt.IsNil)
	parsedToken, err := jwt.ParseInsecure([]byte(decodedToken))
	c.Assert(err, qt.IsNil)
	c.Assert(parsedToken.Subject(), qt.Equals, "user-foo@canonical.com")
}

func TestLoginClientCredentials(t *testing.T) {
	c := qt.New(t)

	j := jimmtest.NewJIMM(c, nil)

	ctx := context.Background()
	invalidClientID := "123@123@"
	_, err := j.LoginClientCredentials(ctx, invalidClientID, "foo-secret")
	c.Assert(err, qt.ErrorMatches, "invalid client ID")

	validClientID := "my-svc-acc"
	user, err := j.LoginClientCredentials(ctx, validClientID, "foo-secret")
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "my-svc-acc@serviceaccount")
}

func TestLoginWithSessionToken(t *testing.T) {
	c := qt.New(t)

	j := jimmtest.NewJIMM(c, nil)

	ctx := context.Background()

	token, err := jwt.NewBuilder().
		Subject("alice@canonical.com").
		Build()
	c.Assert(err, qt.IsNil)
	serialisedToken, err := jwt.NewSerializer().Serialize(token)
	c.Assert(err, qt.IsNil)
	b64Token := base64.StdEncoding.EncodeToString(serialisedToken)

	_, err = j.LoginWithSessionToken(ctx, "invalid-token")
	c.Assert(err, qt.ErrorMatches, "failed to decode token")

	user, err := j.LoginWithSessionToken(ctx, b64Token)
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "alice@canonical.com")
}

func TestLoginWithSessionCookie(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	_, err := j.LoginWithSessionCookie(ctx, "")
	c.Assert(err, qt.ErrorMatches, "missing cookie identity")

	user, err := j.LoginWithSessionCookie(ctx, "alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "alice@canonical.com")
}
