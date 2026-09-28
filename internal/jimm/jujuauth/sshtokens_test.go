// Copyright 2025 Canonical.

package jujuauth_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
)

func TestNewSSHToken(t *testing.T) {
	c := qt.New(t)

	jwtSvc := testJWTService{}
	authFactory := jujuauth.NewFactory(nil, &jwtSvc, nil)
	sshTokenGen := authFactory.NewSSHGenerator()

	params := jujuauth.SSHTokenArgs{
		User:           "testuser",
		ControllerUUID: "123",
		ModelTag:       names.NewModelTag("testmodel"),
	}
	sshToken, err := sshTokenGen.NewSSHToken(context.Background(), params)
	c.Assert(err, qt.IsNil)
	c.Assert(string(sshToken), qt.Equals, "test jwt")

	c.Assert(jwtSvc.params.User, qt.Equals, "testuser")
	c.Assert(jwtSvc.params.Controller, qt.Equals, "123")
	c.Assert(jwtSvc.params.ExtraClaims, qt.IsNil)
	c.Assert(jwtSvc.params.Access, qt.DeepEquals, map[string]string{
		"model-testmodel": string(permission.AdminAccess),
	})
}
