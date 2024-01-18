// Copyright 2024 canonical.

package auth_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"testing"

	"github.com/canonical/jimm/internal/auth"
	"github.com/coreos/go-oidc/v3/oidc"
	qt "github.com/frankban/quicktest"
)

func TestDevice(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:      "http://localhost:8082/realms/jimm",
		DeviceClientID: "jimm-device",
		DeviceScopes:   []string{oidc.ScopeOpenID, "profile", "email"},
	})
	c.Assert(err, qt.IsNil)

	res, err := authSvc.Device(ctx)
	c.Assert(err, qt.IsNil)

	jar, err := cookiejar.New(nil)
	c.Assert(err, qt.IsNil)

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Println("redirected to", req.URL)
			return nil
		},
	}

	// Post login form
	verResp, err := client.Get(res.VerificationURIComplete)
	c.Assert(err, qt.IsNil)
	defer verResp.Body.Close()
	b, err := io.ReadAll(verResp.Body)
	c.Assert(err, qt.IsNil)

	re := regexp.MustCompile(`action="(.*?)" method=`)
	match := re.FindStringSubmatch(string(b))
	loginForm := match[1]

	v := url.Values{}
	v.Add("username", "jimm-test")
	v.Add("password", "password")
	loginResp, err := client.PostForm(loginForm, v)
	c.Assert(err, qt.IsNil)
	defer loginResp.Body.Close()

	// Post consent
	b, err = io.ReadAll(loginResp.Body)
	c.Assert(err, qt.IsNil)

	re = regexp.MustCompile(`action="(.*?)" method=`)
	match = re.FindStringSubmatch(string(b))
	consentForm := match[1]
	v = url.Values{}
	v.Add("accept", "Yes")
	consentResp, err := client.PostForm("http://localhost:8082"+consentForm, v)
	c.Assert(err, qt.IsNil)
	defer consentResp.Body.Close()

	// Read consent resp
	b, err = io.ReadAll(consentResp.Body)
	c.Assert(err, qt.IsNil)
	// TODO check page contains "Device Login Successful"

	token, err := authSvc.DeviceAccessToken(ctx, res)
	c.Assert(err, qt.IsNil)
	c.Assert(token, qt.IsNotNil)

	idToken, err := authSvc.ExtractAndVerifyIDToken(ctx, token)
	c.Assert(err, qt.IsNil)
	c.Assert(idToken, qt.IsNotNil)

	// Test subject set
	c.Assert(idToken.Subject, qt.Equals, "8281cec3-5b48-46eb-a41d-72c15ec3f9e0")

	// Retrieve the email
	email, err := authSvc.Email(idToken)
	c.Assert(err, qt.IsNil)
	c.Assert(email, qt.Equals, "jimm-test@canonical.com")
}
