// Copyright 2024 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"testing"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/jujuapi"
	"github.com/canonical/jimm/internal/openfga"
	qt "github.com/frankban/quicktest"
)

func TestAddServiceAccount(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about             string
		addServiceAccount func(ctx context.Context, user *openfga.User, clientID string) error
		args              params.AddServiceAccountRequest
		expectedError     string
	}{{
		about: "Valid client ID",
		addServiceAccount: func(ctx context.Context, user *openfga.User, clientID string) error {
			return nil
		},
		args: params.AddServiceAccountRequest{
			ID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		},
	}, {
		about: "Invalid Client ID",
		addServiceAccount: func(ctx context.Context, user *openfga.User, clientID string) error {
			return nil
		},
		args: params.AddServiceAccountRequest{
			ID: "_123_",
		},
		expectedError: "invalid client ID",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			jimm := &jimmtest.JIMM{
				AddServiceAccount_: test.addServiceAccount,
			}
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})

			err := cr.AddServiceAccount(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
