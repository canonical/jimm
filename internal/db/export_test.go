// Copyright 2025 Canonical.

package db

import (
	"context"
	"embed"
)

var (
	NewUUID            = &newUUID
	MigrationTableName = migrationTableName
	JobLogLockQuery    = &jobLoglockQuery
)

func (d *Database) MigrateFromSource(ctx context.Context, fs embed.FS, sqlPath string) error {
	return d.migrateFromSource(ctx, fs, sqlPath)
}
