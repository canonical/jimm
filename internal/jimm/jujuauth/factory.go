// Copyright 2024 Canonical.
package jujuauth

type Factory struct {
	db            GeneratorDatabase
	jwtService    JWTService
	accessChecker GeneratorAccessChecker
}

func NewFactory(db GeneratorDatabase, jwtService JWTService, accessChecker GeneratorAccessChecker) *Factory {
	return &Factory{
		db:            db,
		jwtService:    jwtService,
		accessChecker: accessChecker,
	}
}

func (f *Factory) New() TokenGenerator {
	return New(f.db, f.accessChecker, f.jwtService)
}
