// Copyright 2024 Canonical.

// Package db contains routines to store and retrieve data from a database.
package db

import (
	"context"
	"database/sql"
	"embed"
	stderr "errors"
	"fmt"
	"path"
	"sync/atomic"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/logger"
)

// Use a custom table name so that we don't run into collisions when OpenFGA or other tools
// are using the same DB as JIMM in our Docker Compose setup.
const (
	migrationTableName = "jimm_schema_migrations"
)

// A Database provides access to the database model. A Database instance
// is safe to use from multiple goroutines.
type Database struct {
	// DB contains the gorm database storing the data.
	DB *gorm.DB

	// migrated holds whether the database has been successfully migrated
	// to the current database version. The value of migrated should always
	// be read using atomic.LoadUint32 and will contain a 0 if the
	// migration is yet to be run, or 1 if it has been run successfully.
	migrated uint32
}

// Transaction starts a new transaction using the database. This allows
// a set of changes to be performed as a single atomic unit. All of the
// transaction steps should be performed in the given function, if this
// function returns an error then all changes in the transaction will be
// aborted and the error returned. Transactions may be nested.
//
// Attempting to start a transaction on an unmigrated database will result
// in an error with a code of errors.CodeUpgradeInProgress.
func (d *Database) Transaction(f func(*Database) error) error {
	if err := d.ready(); err != nil {
		return err
	}
	return d.DB.Transaction(func(tx *gorm.DB) error {
		d := *d
		d.DB = tx
		return f(&d)
	})
}

// Migrate migrates the configured database to have the structure required
// by the current data model. If the database is not configured then an error
// with a code of errors.CodeServerConfiguration will be returned.
func (d *Database) Migrate(ctx context.Context) error {
	const op = errors.Op("db.Migrate")
	if d == nil || d.DB == nil {
		return errors.E(op, errors.CodeServerConfiguration, "database not configured")
	}

	err := d.migrateFromSource(ctx, dbmodel.SQL, path.Join("sql", d.DB.Name()))
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (d *Database) migrateFromSource(ctx context.Context, fs embed.FS, sqlPath string) error {
	sqlDir, err := iofs.New(fs, sqlPath)
	if err != nil {
		return fmt.Errorf("unable to create new sql filesys: %w", err)
	}

	db := d.DB.WithContext(ctx)
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to obtain raw DB: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to obtain DB conn: %w", err)
	}

	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{MigrationsTable: migrationTableName})
	if err != nil {
		return fmt.Errorf("unable to create new driver instance: %w", err)
	}

	// DB name is left blank because it is contained in the driver/DB connection.
	m, err := migrate.NewWithInstance("iofs", sqlDir, "", driver)
	if err != nil {
		return fmt.Errorf("unable to create new migrator: %w", err)
	}
	defer m.Close()

	// Setup custom logger for consistent output.
	logger := logger.MigrationLogger{Logger: zapctx.Logger(ctx)}
	m.Log = logger

	if err := d.handleDeprecatedMigrations(ctx, m); err != nil {
		return fmt.Errorf("failed to handle deprecated migrations: %w", err)
	}

	v, dirty, err := m.Version()
	if err != nil {
		if !stderr.Is(err, migrate.ErrNilVersion) {
			return fmt.Errorf("failed to get db version: %w", err)
		}
	}

	if dirty {
		// nolint:gosec
		workingVersion := int(v) - 1
		zapctx.Info(ctx, "dirty database, reverting version", zap.Int("version", workingVersion))
		if err := m.Force(workingVersion); err != nil {
			return fmt.Errorf("failed to fix dirty db version: %w", err)
		}
	}

	if err := m.Up(); err != nil {
		if !stderr.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("failed to migrate db: %w", err)
		}
	}

	atomic.StoreUint32(&d.migrated, 1)
	return nil
}

// This method is used for handling deployments that are live when we made the
// switch from a home-grown migration library to golang-migrate. To avoid running
// migrations twice, we check if the old "versions" table exists and make golang-migrate
// aware of which migrations have been run using the Force() method.
func (d *Database) handleDeprecatedMigrations(ctx context.Context, m *migrate.Migrate) error {
	var version int
	err := d.DB.Raw("SELECT minor FROM versions;").Row().Scan(&version)
	if err != nil {
		// The versions table may already be deleted. Other errors are ignored intentionally.
		zapctx.Debug(ctx, "no minor version from deprecated migrations table", zap.Error(err))
		//nolint:nilerr
		return nil
	}
	if version == 0 {
		return nil
	}
	zapctx.Debug(ctx, "forcing db version", zap.Int("version", version))
	err = m.Force(version)
	if err != nil {
		return err
	}
	return nil
}

// ready checks that the database is ready to accept requests. An error is
// returned if the database is not yet initialised.
func (d *Database) ready() error {
	if d == nil || d.DB == nil {
		return errors.E(errors.CodeServerConfiguration, "database not configured")
	}
	if atomic.LoadUint32(&d.migrated) == 0 {
		return errors.E(errors.CodeUpgradeInProgress)
	}
	return nil
}

// Close closes open connections to the underlying database backend.
func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return errors.E(err, "failed to get the internal DB object")
	}
	if err := sqlDB.Close(); err != nil {
		return errors.E(err, "failed to close database connection")
	}
	return nil
}

// Now returns the current time as a valid sql.NullTime. The time that is
// returned is in UTC and is truncated to milliseconds which is the
// resolution supported on all databases.
func Now() sql.NullTime {
	return sql.NullTime{
		Time:  time.Now().UTC().Truncate(time.Millisecond),
		Valid: true,
	}
}
