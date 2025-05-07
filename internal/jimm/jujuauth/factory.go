// Copyright 2025 Canonical.

package jujuauth

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

// NewSSHGenerator returns a new token generator for Juju SSH connections.
// The SSHToken generator is not stateful and can be re-used across
// multiple connections.
func (f *Factory) NewSSHGenerator() SSHTokenGenerator {
	return newSSHTokenGenerator(f.jwtService)
}
