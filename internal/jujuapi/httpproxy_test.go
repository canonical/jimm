// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func TestAuthenticate(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	jimmMock := jimmtest.JIMM{
		LoginService: mocks.LoginService{
			LoginWithSessionToken_: func(ctx context.Context, sessionToken string) (*openfga.User, error) {
				if sessionToken == "valid_jwt" {
					return &openfga.User{}, nil
				} else {
					return nil, errors.New("invalid JWT")
				}

			},
		},
		FetchIdentity_: func(ctx context.Context, username string) (*openfga.User, error) {
			return &openfga.User{}, nil
		},
		GetUserModelAccess_: func(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error) {
			if model.Id() == "54d9f921-c45a-4825-8253-74e7edc28067" {
				return "", errors.New("not auth")
			}
			return "admin", nil
		},
	}

	tests := []struct {
		description  string
		token        string
		path         string
		errorMessage string
	}{
		{
			description:  "no auth",
			token:        "",
			path:         "/",
			errorMessage: "authentication missing",
		},
		{
			description:  "wrong token",
			token:        "asda",
			path:         "/",
			errorMessage: "invalid JWT",
		},
		{
			description:  "wrong path",
			token:        "valid_jwt",
			path:         "/",
			errorMessage: "cannot parse path",
		},
		{
			description:  "not authorized user on this model",
			token:        "valid_jwt",
			path:         "/54d9f921-c45a-4825-8253-74e7edc28067/charms",
			errorMessage: "unauthorized",
		},
		{
			description:  "authorized user on this model",
			token:        "valid_jwt",
			path:         "/54d9f921-c45a-4825-8253-74e7edc28066/charms",
			errorMessage: "",
		},
	}
	httpProxier := jujuapi.HTTPProxier(&jimmMock)
	for _, test := range tests {
		c.Run(test.description, func(c *qt.C) {
			req, err := http.NewRequest("GET", test.path, nil)
			c.Assert(err, qt.Equals, nil)
			if test.token != "" {
				req.SetBasicAuth("", test.token)
			}
			err = httpProxier.Authenticate(ctx, nil, req)
			if test.errorMessage != "" {
				c.Assert(err, qt.ErrorMatches, test.errorMessage)
			} else {
				c.Assert(err, qt.IsNil)
			}
		})
	}

}
