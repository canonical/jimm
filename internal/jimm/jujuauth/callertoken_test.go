// Copyright 2026 Canonical.

package jujuauth_test

import (
	"encoding/json"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// testCallerTokenEnv defines a controller with a cloud, a non-admin user
// (bob) with can_addmodel on the controller and writer on a model, and
// an admin user (alice) with administrator on the controller and model.
//nolint:gosec // Test data, not real credentials.
const testCallerTokenEnv = `clouds:
- name: test-cloud
  type: lxd
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test-cloud
  owner: alice@canonical.com
  type: empty
users:
- username: bob@canonical.com
- username: alice@canonical.com
controllers:
- name: test-controller
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region
  agent-version: 3.6.24
  users:
  - user: bob@canonical.com
    access: add-model
  - user: alice@canonical.com
    access: admin
models:
- name: test-model
  uuid: 00000002-0000-0000-0000-000000000002
  controller: test-controller
  owner: alice@canonical.com
  cloud: test-cloud
  region: test-region
  cloud-credential: test-cred
  users:
  - user: bob@canonical.com
    access: write
  - user: alice@canonical.com
    access: admin
application-offers:
- model-name: test-model
  model-owner: alice@canonical.com
  name: test-offer
  uuid: 00000003-0000-0000-0000-000000000003
  url: alice/test-model.test-offer
  users:
  - user: bob@canonical.com
    access: consume
`

// TestCallerScopedLoginTokenForNonAdminUser verifies that NewCallerScopedLoginToken
// mints a JWT carrying the caller's real OpenFGA-derived permissions
// (controller: login, cloud: add-model, model: write).
func TestCallerScopedLoginTokenForNonAdminUser(t *testing.T) {
	c := qt.New(t)
	env := jimmtest.SetupJimmEnv(c)
	ctx := c.Context()

	testEnv := jimmtest.ParseEnvironment(c, testCallerTokenEnv)
	testEnv.PopulateDBAndPermissions(c, env.JIMM.ResourceTag(), env.JIMM.Database, env.OFGAClient)

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = env.JIMM.Database.GetIdentity(ctx, bobIdentity)
	c.Assert(err, qt.IsNil)
	bob := env.NewUser(bobIdentity)

	controllerTag := names.NewControllerTag("00000001-0000-0000-0000-000000000001")
	modelTag := names.NewModelTag("00000002-0000-0000-0000-000000000002")
	ctl := &dbmodel.Controller{UUID: controllerTag.Id()}
	err = env.JIMM.Database.GetController(ctx, ctl)
	c.Assert(err, qt.IsNil)

	token, err := env.JIMM.JujuAuthFactory.NewCallerScopedLoginToken(ctx, []names.Tag{modelTag}, ctl, bob)
	c.Assert(err, qt.IsNil)

	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)
	c.Assert(parsed.Subject(), qt.Equals, bobIdentity.ResourceTag().String())

	access := decodeAccess(c, parsed)

	c.Assert(access[controllerTag.String()], qt.Equals, "login",
		qt.Commentf("non-admin user must not receive controller superuser access"))
	c.Assert(access[modelTag.String()], qt.Equals, "write",
		qt.Commentf("model access must reflect the user's real OpenFGA permission"))

	for tag, level := range access {
		if level == "superuser" {
			c.Fatalf("non-admin user received superuser access on %s", tag)
		}
	}
}

// TestCallerLoginTokenUsesCompatibilityTokenForOldController verifies that
// NewCallerLoginToken falls back to the superuser token for controllers
// older than 3.6.24.
func TestCallerLoginTokenUsesCompatibilityTokenForOldController(t *testing.T) {
	c := qt.New(t)
	env := jimmtest.SetupJimmEnv(c)
	ctx := c.Context()

	identity, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	user := &openfga.User{Identity: identity}
	controllerUUID := uuid.New().String()
	controllerTag := names.NewControllerTag(controllerUUID)
	modelTag := names.NewModelTag(uuid.New().String())

	for _, version := range []string{"", "not-a-version", "3.6.23"} {
		t.Run(version, func(t *testing.T) {
			c := qt.New(t)
			token, err := env.JIMM.JujuAuthFactory.NewCallerLoginToken(
				ctx,
				[]names.Tag{modelTag, names.NewApplicationOfferTag(uuid.New().String())},
				&dbmodel.Controller{UUID: controllerUUID, AgentVersion: version},
				user,
			)
			c.Assert(err, qt.IsNil)

			parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
			c.Assert(err, qt.IsNil)
			c.Assert(parsed.Subject(), qt.Equals, user.Tag().String())
			access := decodeAccess(c, parsed)
			c.Assert(access, qt.DeepEquals, map[string]string{
				controllerTag.String(): "superuser",
				modelTag.String():      "admin",
			})
		})
	}
}

// TestCallerLoginTokenUsesCallerScopedTokenForNewController verifies that
// NewCallerLoginToken routes to the caller-scoped path (not the superuser
// fallback) for controllers >= 3.6.24.
func TestCallerLoginTokenUsesCallerScopedTokenForNewController(t *testing.T) {
	c := qt.New(t)
	env := jimmtest.SetupJimmEnv(c)
	ctx := c.Context()

	testEnv := jimmtest.ParseEnvironment(c, testCallerTokenEnv)
	testEnv.PopulateDBAndPermissions(c, env.JIMM.ResourceTag(), env.JIMM.Database, env.OFGAClient)

	aliceIdentity, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	err = env.JIMM.Database.GetIdentity(ctx, aliceIdentity)
	c.Assert(err, qt.IsNil)
	alice := env.NewUser(aliceIdentity)

	controllerTag := names.NewControllerTag("00000001-0000-0000-0000-000000000001")
	modelTag := names.NewModelTag("00000002-0000-0000-0000-000000000002")
	ctl := &dbmodel.Controller{UUID: controllerTag.Id()}
	err = env.JIMM.Database.GetController(ctx, ctl)
	c.Assert(err, qt.IsNil)

	token, err := env.JIMM.JujuAuthFactory.NewCallerLoginToken(
		ctx, []names.Tag{modelTag}, ctl, alice,
	)
	c.Assert(err, qt.IsNil)

	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)
	c.Assert(parsed.Subject(), qt.Equals, alice.Tag().String())

	access := decodeAccess(c, parsed)

	c.Assert(access[controllerTag.String()], qt.Equals, "superuser",
		qt.Commentf("admin on a >=3.6.24 controller must receive real controller access"))
	c.Assert(access[modelTag.String()], qt.Equals, "admin",
		qt.Commentf("model admin on a >=3.6.24 controller must receive real model access"))
}

func TestCallerLoginTokenAllowsControllerOnlyCompatibilityToken(t *testing.T) {
	c := qt.New(t)
	env := jimmtest.SetupJimmEnv(c)
	ctx := c.Context()

	identity, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	user := &openfga.User{Identity: identity}
	controllerUUID := uuid.New().String()
	controllerTag := names.NewControllerTag(controllerUUID)

	token, err := env.JIMM.JujuAuthFactory.NewCallerLoginToken(
		ctx, nil,
		&dbmodel.Controller{UUID: controllerUUID, AgentVersion: "3.6.23"},
		user,
	)
	c.Assert(err, qt.IsNil)

	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)
	c.Assert(decodeAccess(c, parsed), qt.DeepEquals, map[string]string{
		controllerTag.String(): "superuser",
	})
}

// TestCallerScopedLoginTokenIncludesApplicationOfferAccess verifies that
// NewCallerScopedLoginToken resolves application-offer tags via OpenFGA
// and includes the resulting access level in the JWT.
func TestCallerScopedLoginTokenIncludesApplicationOfferAccess(t *testing.T) {
	c := qt.New(t)
	env := jimmtest.SetupJimmEnv(c)
	ctx := c.Context()

	testEnv := jimmtest.ParseEnvironment(c, testCallerTokenEnv)
	testEnv.PopulateDBAndPermissions(c, env.JIMM.ResourceTag(), env.JIMM.Database, env.OFGAClient)

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = env.JIMM.Database.GetIdentity(ctx, bobIdentity)
	c.Assert(err, qt.IsNil)
	bob := env.NewUser(bobIdentity)

	controllerTag := names.NewControllerTag("00000001-0000-0000-0000-000000000001")
	offerTag := names.NewApplicationOfferTag("00000003-0000-0000-0000-000000000003")
	ctl := &dbmodel.Controller{UUID: controllerTag.Id()}
	err = env.JIMM.Database.GetController(ctx, ctl)
	c.Assert(err, qt.IsNil)

	token, err := env.JIMM.JujuAuthFactory.NewCallerScopedLoginToken(
		ctx, []names.Tag{offerTag}, ctl, bob,
	)
	c.Assert(err, qt.IsNil)

	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)
	c.Assert(parsed.Subject(), qt.Equals, bobIdentity.ResourceTag().String())

	access := decodeAccess(c, parsed)

	c.Assert(access[controllerTag.String()], qt.Equals, "login")
	c.Assert(access[offerTag.String()], qt.Equals, "consume",
		qt.Commentf("offer access must reflect the user's real OpenFGA permission"))
}

// decodeAccess extracts the "access" claim from a parsed JWT as a
// map[string]string.
func decodeAccess(c *qt.C, parsed jwt.Token) map[string]string {
	accessRaw, ok := parsed.Get("access")
	c.Assert(ok, qt.IsTrue, qt.Commentf("JWT missing access claim"))
	accessBytes, err := json.Marshal(accessRaw)
	c.Assert(err, qt.IsNil)
	var access map[string]string
	err = json.Unmarshal(accessBytes, &access)
	c.Assert(err, qt.IsNil)
	return access
}
