// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

func TestAuthenticate(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	bob := openfga.NewUser(bobIdentity, ofgaClient)
	validModelUUID := "54d9f921-c45a-4825-8253-74e7edc28066"
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(bob.ResourceTag()),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag(validModelUUID)),
	}
	err = ofgaClient.AddRelation(ctx, tuple)
	c.Assert(err, qt.IsNil)
	jimmMock := jimmtest.JIMM{
		LoginService: mocks.LoginService{
			LoginWithSessionToken_: func(ctx context.Context, sessionToken string) (*openfga.User, error) {
				if sessionToken == "valid_jwt" {
					return bob, nil
				} else {
					return nil, errors.New("invalid JWT")
				}

			},
		},
		FetchIdentity_: func(ctx context.Context, username string) (*openfga.User, error) {
			return &openfga.User{}, nil
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
			path:         "/" + validModelUUID + "/charms",
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
