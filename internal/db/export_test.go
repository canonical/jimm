// Copyright 2025 Canonical.

package db

import (
	"context"
	"embed"
)

var (
	JwksKind                   = jwksKind
	JwksPublicKeyTag           = jwksPublicKeyTag
	JwksPrivateKeyTag          = jwksPrivateKeyTag
	JwksExpiryTag              = jwksExpiryTag
	OAuthKind                  = oauthKind
	OAuthKeyTag                = oauthKeyTag
	OAuthSessionStoreSecretTag = oauthSessionStoreSecretTag
	NewUUID                    = &newUUID
	MigrationTableName         = migrationTableName
)

func (d *Database) MigrateFromSource(ctx context.Context, fs embed.FS, sqlPath string) error {
	return d.migrateFromSource(ctx, fs, sqlPath)
}
