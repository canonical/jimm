// Copyright 2025 Canonical.

package jujuauth

import (
	"context"
	"fmt"

	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// minJujuVersionForCallerToken is the first Juju controller version that
// correctly honours model-level access in a JWT during permission checks.
// Controllers older than this version incorrectly reject tokens that carry
// model-admin access without a controller-superuser claim, so a superuser
// fallback token must be minted for them instead.
//
// TODO(luci1900): Once the minimum supported Juju controller version
// exceeds this boundary, remove the superuser fallback in
// NewCallerLoginToken and makeSuperuserToken entirely.
var minJujuVersionForCallerToken = version.MustParse("3.6.24")

// Factory holds the necessary components for producing
// Juju authenticator objects. Currently a login token generator
// and an SSH token generator are available.
type Factory struct {
	db            GeneratorDatabase
	jwtService    JWTService
	accessChecker GeneratorAccessChecker
}

// NewFactory returns a new factory object.
func NewFactory(db GeneratorDatabase, jwtService JWTService, accessChecker GeneratorAccessChecker) *Factory {
	return &Factory{
		db:            db,
		jwtService:    jwtService,
		accessChecker: accessChecker,
	}
}

// NewLoginGenerator returns a new token generator for Juju RPC login requests.
// The LoginTokenGenerator is stateful and should be re-used for the lifetime
// of a single connection, and recreated for each new connection.
func (f *Factory) NewLoginGenerator() LoginTokenGenerator {
	return newLoginTokenGenerator(f.db, f.accessChecker, f.jwtService)
}

// NewLoginToken returns a Juju login token for the given user, model, and controller.
//
// This is a convenience method that wraps the creation of a LoginTokenGenerator and
// the generation of a login token in one step. This is useful for scenarios where we
// don't have a long lived connection that may need multiple tokens.
func (f *Factory) NewLoginToken(ctx context.Context, modelTag names.ModelTag, controllerTag names.ControllerTag, user *openfga.User) ([]byte, error) {
	generator := f.NewLoginGenerator()
	generator.SetTags(modelTag, controllerTag)
	return generator.MakeLoginToken(ctx, user)
}

// NewSuperuserLoginToken creates a login token for the provided user with controller superuser and model admin permissions.
//
// NB: Avoid using method and prefer NewLoginToken to mint a token with the user's real perwmissions.
//
// This is only used as a fallback for avoiding a bug in Juju 3.6.23 and below where Juju does not check the model admin
// permission for a JWT.
func (f *Factory) NewSuperuserLoginToken(ctx context.Context, modelTag names.ModelTag, controllerTag names.ControllerTag, user *openfga.User) ([]byte, error) {
	generator := f.NewLoginGenerator()
	generator.SetTags(modelTag, controllerTag)
	var resourceTags []names.Tag
	if modelTag.Id() != "" {
		resourceTags = []names.Tag{modelTag}
	}
	return generator.makeSuperuserToken(ctx, user, resourceTags)
}

// NewCallerScopedLoginToken mints a login token carrying the caller's
// real access for the given user on the specified controller. If the
// controller is already persisted, its CloudRegions are fetched from the
// database so cloud-access claims can be resolved; otherwise the
// passed-in controller is used as-is.
//
// resourceTags may contain any mix of model and application-offer tags
// whose access levels should be embedded in the token.
func (f *Factory) NewCallerScopedLoginToken(ctx context.Context, resourceTags []names.Tag, ctl *dbmodel.Controller, user *openfga.User) ([]byte, error) {
	ctlWithClouds := dbmodel.Controller{}
	ctlWithClouds.SetTag(ctl.ResourceTag())
	if err := f.db.GetController(ctx, &ctlWithClouds); err != nil {
		if errors.ErrorCode(err) != errors.CodeNotFound {
			return nil, fmt.Errorf("failed to fetch controller for caller token: %w", err)
		}
		ctlWithClouds = *ctl
	}
	accessMap, err := buildAccessMap(ctx, user, resourceTags, ctl.ResourceTag(), ctlWithClouds, f.accessChecker)
	if err != nil {
		return nil, err
	}
	return f.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: ctl.ResourceTag().Id(),
		User:       user.Tag().String(),
		Access:     accessMap,
	})
}

// NewCallerLoginToken returns a login token suitable for a real user's
// call to the specified controller. Older Juju controllers do not
// correctly honour model-level JWT claims, so unknown and older versions
// use the compatibility superuser token.
func (f *Factory) NewCallerLoginToken(ctx context.Context, resourceTags []names.Tag, ctl *dbmodel.Controller, user *openfga.User) ([]byte, error) {
	ctrlVersion, err := version.Parse(ctl.AgentVersion)
	if err != nil || ctrlVersion.Compare(minJujuVersionForCallerToken) < 0 {
		generator := f.NewLoginGenerator()
		generator.SetTags(names.ModelTag{}, ctl.ResourceTag())
		return generator.makeSuperuserToken(ctx, user, resourceTags)
	}
	return f.NewCallerScopedLoginToken(ctx, resourceTags, ctl, user)
}

// NewSSHGenerator returns a new token generator for Juju SSH connections.
// The SSHToken generator is not stateful and can be re-used across
// multiple connections.
func (f *Factory) NewSSHGenerator() SSHTokenGenerator {
	return newSSHTokenGenerator(f.jwtService)
}
