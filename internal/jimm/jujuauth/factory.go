// Copyright 2025 Canonical.

package jujuauth

// Factory holds the necessary components for producing new stateful
// Juju authenticator objects. Because these objects are
// stateful, it is expected that a new one is used for each connection.
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

// New returns a new Juju token generator.
func (f *Factory) New() TokenGenerator {
	return newTokenGenerator(f.db, f.accessChecker, f.jwtService)
}
