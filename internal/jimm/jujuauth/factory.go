// Copyright 2025 Canonical.

package jujuauth

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/openfga"
)

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
	return generator.makeSuperuserToken(ctx, user)
}

// NewSSHGenerator returns a new token generator for Juju SSH connections.
// The SSHToken generator is not stateful and can be re-used across
// multiple connections.
func (f *Factory) NewSSHGenerator() SSHTokenGenerator {
	return newSSHTokenGenerator(f.jwtService)
}
