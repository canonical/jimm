// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"embed"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	qt "github.com/frankban/quicktest"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

//go:embed testdata/invalidmigrations/*.sql
var invalidSQL embed.FS

//go:embed testdata/validmigrations/*.sql
var validSQL embed.FS

func newTestMigrator(c *qt.C, d *db.Database, fs embed.FS, sqlPath string) *migrate.Migrate {
	sqlDir, err := iofs.New(fs, sqlPath)
	c.Assert(err, qt.IsNil)

	sqlDB, err := d.DB.DB()
	c.Assert(err, qt.IsNil)

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{MigrationsTable: db.MigrationTableName})
	c.Assert(err, qt.IsNil)

	m, err := migrate.NewWithInstance("iofs", sqlDir, "", driver)
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() { m.Close() })

	return m
}

// dbSuite contains a suite of database tests that are run against
// different database engines.
type dbSuite struct {
	Database *db.Database
}

func (s *dbSuite) TestMigrate(c *qt.C) {
	// Migrate from an empty database should work.
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	// Attempting to migrate to the version that is already there should
	// also work.
	err = s.Database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
}

// TestMigrationRanges verifies that migrations below 1000 can be applied first,
// followed by the complete migration set. This models upgrading a v3 database
// before applying the v4-only 1000+ migrations.
func (s *dbSuite) TestMigrationRanges(c *qt.C) {
	ctx := context.Background()
	lowMigrations, err := migrationsBelow(1000)
	c.Assert(err, qt.IsNil)

	err = s.Database.MigrateFromSource(ctx, lowMigrations, "migrations")
	c.Assert(err, qt.IsNil)

	// The complete set must then apply the v4-only migrations successfully.
	err = s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)
}

func migrationsBelow(limit int) (fs.FS, error) {
	result := fstest.MapFS{}
	err := fs.WalkDir(dbmodel.SQL, "sql/postgres", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return err
		}

		base := strings.TrimSuffix(filepath.Base(path), ".up.sql")
		versionText, _, ok := strings.Cut(base, "_")
		if !ok {
			return nil
		}
		version, err := strconv.Atoi(versionText)
		if err != nil {
			return err
		}
		if version >= limit {
			return nil
		}

		contents, err := fs.ReadFile(dbmodel.SQL, path)
		if err != nil {
			return err
		}
		result[filepath.Join("migrations", filepath.Base(path))] = &fstest.MapFile{Data: contents}
		return nil
	})
	return result, err
}

// TestFailedMigration verifies a failed migration will cause a dirty migration
// that when fixed should automatically work.
func (s *dbSuite) TestFailedMigration(c *qt.C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.Database.MigrateFromSource(ctx, invalidSQL, "testdata/invalidmigrations")
	c.Assert(err, qt.Not(qt.IsNil))

	m := newTestMigrator(c, s.Database, invalidSQL, "testdata/invalidmigrations")
	v, dirty, err := m.Version()
	c.Assert(err, qt.IsNil)
	c.Assert(dirty, qt.IsTrue)
	c.Assert(v, qt.Equals, uint(2))

	err = s.Database.MigrateFromSource(ctx, validSQL, "testdata/validmigrations")
	c.Assert(err, qt.IsNil)

	m = newTestMigrator(c, s.Database, validSQL, "testdata/validmigrations")
	v, dirty, err = m.Version()
	c.Assert(err, qt.IsNil)
	c.Assert(dirty, qt.IsFalse)
	c.Assert(v, qt.Equals, uint(3))
}

func TestMigrateUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var database db.Database
	err := database.Migrate(context.Background())
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func TestTransactionUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var database db.Database
	err := database.Transaction(func(d *db.Database) error {
		return errors.New("unexpected function call")
	})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestTransaction(c *qt.C) {
	err := s.Database.Transaction(func(d *db.Database) error {
		return errors.New("unexpected function call")
	})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.Transaction(func(d *db.Database) error {
		c.Check(d, qt.Not(qt.Equals), s.Database)
		return d.GetIdentity(context.Background(), i)
	})
	c.Assert(err, qt.IsNil)

	err = s.Database.Transaction(func(d *db.Database) error {
		return errors.New("test error")
	})
	c.Check(err, qt.ErrorMatches, `test error`)
}
