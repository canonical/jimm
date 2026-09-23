// Copyright 2025 Canonical.

package db

import (
	"context"
	"io/fs"
)

var (
	NewUUID            = &newUUID
	MigrationTableName = migrationTableName
	JobLogLockQuery    = &jobLoglockQuery
)

func (d *Database) MigrateFromSource(ctx context.Context, migrationFS fs.FS, sqlPath string) error {
	return d.migrateFromSource(ctx, migrationFS, sqlPath)
}
