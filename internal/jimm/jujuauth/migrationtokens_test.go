// Copyright 2025 Canonical.

package jujuauth_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
)

func TestNewMigrationToken(t *testing.T) {
	c := qt.New(t)

	jwtSvc := testJWTService{}
	authFactory := jujuauth.NewFactory(nil, &jwtSvc, nil)
	migrationTokenGen := authFactory.NewMigrationTokenGenerater("123")

	params := jujuauth.MigrationTokenArgs{
		User:     "testuser",
		ModelTag: names.NewModelTag("testmodel"),
	}
	migrationToken, err := migrationTokenGen.NewToken(context.Background(), params)
	c.Assert(err, qt.IsNil)
	c.Assert(string(migrationToken), qt.Equals, "test jwt")

	c.Assert(jwtSvc.params.Expiry, qt.Equals, 3*time.Hour)
	c.Assert(jwtSvc.params.User, qt.Equals, "testuser")
	c.Assert(jwtSvc.params.Controller, qt.Equals, "123")
	c.Assert(jwtSvc.params.Access, qt.DeepEquals, map[string]string{
		"model-testmodel": string(permission.AdminAccess),
	})
}
