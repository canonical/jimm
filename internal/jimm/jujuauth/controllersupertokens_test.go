// Copyright 2026 Canonical.

package jujuauth_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func TestFactoryNewControllerSuperuserToken(t *testing.T) {
	c := qt.New(t)
	const controllerUUID = "00000001-0000-0000-0000-000000000001"
	const modelUUID = "00000002-0000-0000-0000-000000000001"

	user := openfga.NewUser(&dbmodel.Identity{Name: "alice@canonical.com"}, nil)

	tests := []struct {
		name           string
		modelTag       names.ModelTag
		expectedAccess map[string]string
	}{
		{
			name:     "controller and model",
			modelTag: names.NewModelTag(modelUUID),
			expectedAccess: map[string]string{
				names.NewControllerTag(controllerUUID).String(): "superuser",
				names.NewModelTag(modelUUID).String():           "admin",
			},
		},
		{
			name:     "controller only",
			modelTag: names.ModelTag{},
			expectedAccess: map[string]string{
				names.NewControllerTag(controllerUUID).String(): "superuser",
			},
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			jwtService := &testJWTService{}
			authFactory := jujuauth.NewFactory(nil, jwtService, nil)

			token, err := authFactory.NewControllerSuperuserToken(context.Background(), controllerUUID, test.modelTag, user)
			c.Assert(err, qt.IsNil)
			c.Assert(string(token), qt.Equals, "test jwt")
			c.Assert(jwtService.params, qt.DeepEquals, jimmjwx.JWTParams{
				Controller: controllerUUID,
				User:       names.NewUserTag("alice@canonical.com").String(),
				Access:     test.expectedAccess,
			})
		})
	}
}
