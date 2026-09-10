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
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// TestCallerScopedLoginTokenForNonAdminUser verifies that NewCallerScopedLoginToken
// mints a JWT carrying the caller's real OpenFGA-derived permissions
// (controller: login, cloud: add-model, model: write).
func TestCallerScopedLoginTokenForNonAdminUser(t *testing.T) {
	c := qt.New(t)
	env := jimmtest.SetupJimmEnv(c)
	ctx := c.Context()

	// Create a non-admin user (bob) with can_addmodel on the controller
	// and writer on a model, but NOT administrator on the controller.
	bobEmail := "bob@canonical.com"
	env.AddUser(c, bobEmail)
	bobIdentity, err := dbmodel.NewIdentity(bobEmail)
	c.Assert(err, qt.IsNil)
	err = env.JIMM.Database.GetIdentity(ctx, bobIdentity)
	c.Assert(err, qt.IsNil)
	bob := env.NewUser(bobIdentity)

	// Add a controller to the DB so buildAccessMap can fetch it.
	// A cloud row must exist first to satisfy the controllers_cloud_name_fkey.
	cloudName := "test-cloud"
	err = env.JIMM.Database.AddCloud(ctx, &dbmodel.Cloud{Name: cloudName, Type: "lxd"})
	c.Assert(err, qt.IsNil)
	controllerUUID := uuid.New().String()
	controllerTag := names.NewControllerTag(controllerUUID)
	ctl := &dbmodel.Controller{
		UUID:          controllerUUID,
		Name:          "test-controller",
		CloudName:     cloudName,
		CACertificate: "test-ca-cert",
	}
	err = env.JIMM.Database.AddController(ctx, ctl)
	c.Assert(err, qt.IsNil)

	// Grant bob can_addmodel on the controller (not administrator).
	err = env.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag(bobEmail)),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(controllerTag),
	})
	c.Assert(err, qt.IsNil)

	// Grant bob writer access on a model (only the tag is needed for the
	// OpenFGA permission check; no DB row is required).
	modelUUID := uuid.New().String()
	modelTag := names.NewModelTag(modelUUID)

	err = env.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag(bobEmail)),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(modelTag),
	})
	c.Assert(err, qt.IsNil)

	// Mint a caller-scoped login token for bob.
	factory := env.JIMM.JujuAuthFactory
	token, err := factory.NewCallerScopedLoginToken(ctx, []names.Tag{modelTag}, ctl, bob)
	c.Assert(err, qt.IsNil)

	// Decode the JWT and inspect the access claim.
	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)

	// The "sub" must be bob, not the JIMM admin identity.
	c.Assert(parsed.Subject(), qt.Equals, bobIdentity.ResourceTag().String())

	access := decodeAccess(c, parsed)

	// Controller access must be "login" (the non-admin level), NOT "superuser".
	c.Assert(access[controllerTag.String()], qt.Equals, "login",
		qt.Commentf("non-admin user must not receive controller superuser access"))

	// Model access must be "write" (the granted level), NOT "admin".
	c.Assert(access[modelTag.String()], qt.Equals, "write",
		qt.Commentf("model access must reflect the user's real OpenFGA permission"))

	// The old hardcoded superuser claim must NOT be present anywhere.
	for tag, level := range access {
		if level == "superuser" {
			c.Fatalf("non-admin user received superuser access on %s", tag)
		}
	}
}

// TestCallerScopedLoginTokenForAdminUser verifies that an admin user still
// receives controller superuser access in the caller-scoped token, matching
// the old hardcoded behaviour for the admin case.
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

	aliceEmail := "alice@canonical.com"
	env.AddAdminUser(c, aliceEmail)
	aliceIdentity, err := dbmodel.NewIdentity(aliceEmail)
	c.Assert(err, qt.IsNil)
	err = env.JIMM.Database.GetIdentity(ctx, aliceIdentity)
	c.Assert(err, qt.IsNil)
	alice := env.NewUser(aliceIdentity)

	cloudName := "caller-scoped-cloud"
	err = env.JIMM.Database.AddCloud(ctx, &dbmodel.Cloud{Name: cloudName, Type: "lxd"})
	c.Assert(err, qt.IsNil)
	controllerUUID := uuid.New().String()
	controllerTag := names.NewControllerTag(controllerUUID)
	ctl := &dbmodel.Controller{
		UUID:          controllerUUID,
		Name:          "caller-scoped-controller",
		CloudName:     cloudName,
		CACertificate: "test-ca-cert",
		AgentVersion:  "3.6.24",
	}
	err = env.JIMM.Database.AddController(ctx, ctl)
	c.Assert(err, qt.IsNil)

	modelUUID := uuid.New().String()
	modelTag := names.NewModelTag(modelUUID)

	// Grant alice admin on the controller and model so the caller-scoped
	// token carries real access (not the NoAccess fallback).
	err = env.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag(aliceEmail)),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(controllerTag),
	})
	c.Assert(err, qt.IsNil)
	err = env.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag(aliceEmail)),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(modelTag),
	})
	c.Assert(err, qt.IsNil)

	token, err := env.JIMM.JujuAuthFactory.NewCallerLoginToken(
		ctx, []names.Tag{modelTag}, ctl, alice,
	)
	c.Assert(err, qt.IsNil)

	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)
	c.Assert(parsed.Subject(), qt.Equals, alice.Tag().String())

	access := decodeAccess(c, parsed)

	// The caller-scoped path mints real access, not the superuser fallback.
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

	bobEmail := "bob@canonical.com"
	env.AddUser(c, bobEmail)
	bobIdentity, err := dbmodel.NewIdentity(bobEmail)
	c.Assert(err, qt.IsNil)
	err = env.JIMM.Database.GetIdentity(ctx, bobIdentity)
	c.Assert(err, qt.IsNil)
	bob := env.NewUser(bobIdentity)

	cloudName := "offer-access-cloud"
	err = env.JIMM.Database.AddCloud(ctx, &dbmodel.Cloud{Name: cloudName, Type: "lxd"})
	c.Assert(err, qt.IsNil)
	controllerUUID := uuid.New().String()
	controllerTag := names.NewControllerTag(controllerUUID)
	ctl := &dbmodel.Controller{
		UUID:          controllerUUID,
		Name:          "offer-access-controller",
		CloudName:     cloudName,
		CACertificate: "test-ca-cert",
		AgentVersion:  "3.6.24",
	}
	err = env.JIMM.Database.AddController(ctx, ctl)
	c.Assert(err, qt.IsNil)

	// Grant bob can_addmodel on the controller (maps to "login" access).
	err = env.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag(bobEmail)),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(controllerTag),
	})
	c.Assert(err, qt.IsNil)

	// Grant bob consumer access on an application offer.
	offerUUID := uuid.New().String()
	offerTag := names.NewApplicationOfferTag(offerUUID)
	err = env.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag(bobEmail)),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(offerTag),
	})
	c.Assert(err, qt.IsNil)

	token, err := env.JIMM.JujuAuthFactory.NewCallerScopedLoginToken(
		ctx, []names.Tag{offerTag}, ctl, bob,
	)
	c.Assert(err, qt.IsNil)

	parsed, err := jwt.Parse(token, jwt.WithVerify(false), jwt.WithValidate(false))
	c.Assert(err, qt.IsNil)
	c.Assert(parsed.Subject(), qt.Equals, bobIdentity.ResourceTag().String())

	access := decodeAccess(c, parsed)

	// Controller access must be "login".
	c.Assert(access[controllerTag.String()], qt.Equals, "login")

	// Offer access must reflect the consume relation.
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
