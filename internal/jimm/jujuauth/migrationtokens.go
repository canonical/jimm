// Copyright 2025 Canonical.

package jujuauth

import (
	"context"
	"time"

	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

const (
	migrationExpiry = 3 * time.Hour
)

// MigrationTokenGenerator is responsible for generating migration tokens.
// It uses the JWTService to create a token with specific claims for migration purposes.
// The token includes the user, model tag, JIMM's UUID and an indication that it is for migration.
type MigrationTokenGenerator struct {
	jwtService JWTService
	jimmUUID   string
}

// newMigrationTokenGenerator creates a new MigrationTokenGenerator instance.
// It requires a JWTService to generate the tokens and JIMM's UUID used for the audience claim.
func newMigrationTokenGenerator(jwtService JWTService, jimmUUID string) MigrationTokenGenerator {
	return MigrationTokenGenerator{
		jwtService: jwtService,
		jimmUUID:   jimmUUID,
	}
}

// MigrationTokenArgs holds the arguments required to generate a migration token.
// It includes the user for whom the token is generated and the model tag associated with the migration
type MigrationTokenArgs struct {
	User     string
	ModelTag names.ModelTag
}

// NewToken generates a new migration token with the specified user and model tag.
// The token is valid for a limited time (defined by migrationExpiry) and includes
// claims that indicate the user has admin access to the specified model tag as
// well as an extra claim indicating that this token is for migration purposes.
func (s *MigrationTokenGenerator) NewToken(ctx context.Context, tokenArgs MigrationTokenArgs) ([]byte, error) {
	token, err := s.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: s.jimmUUID,
		User:       tokenArgs.User,
		Access: map[string]string{
			tokenArgs.ModelTag.String(): string(permission.AdminAccess),
		},
		Expiry: migrationExpiry,
	})
	if err != nil {
		return nil, err
	}

	return token, nil
}
