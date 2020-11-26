// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

// gormDB creates a new *gorm.DB for use in tests. The newly created DB
// will be an in-memory SQLite database that logs to the given test with
// debug enabled. If any objects are specified the datbase automatically
// performs the migrations for those objects.
func gormDB(t testing.TB, objects ...interface{}) *gorm.DB {
	cfg := gorm.Config{
		Logger: jimmtest.NewGormLogger(t),
	}
	db, err := gorm.Open(sqlite.Open("file::memory:"), &cfg)
	if err != nil {
		t.Fatalf("error creating test database: %s", err)
	}
	// Enable foreign key constraints in SQLite, which are disabled by
	// default. This makes the encoded foreign key constraints behave as
	// expected.
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatalf("error enabling foreign keys: %s", err)
	}
	err = db.AutoMigrate(objects...)
	if err != nil {
		t.Fatalf("error perform migrations on test database: %s", err)
	}
	return db
}
