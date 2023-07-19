// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
)

// gormDB creates a new *gorm.DB for use in tests. The newly created DB
// will be an in-memory SQLite database that logs to the given test with
// debug enabled. If any objects are specified the datbase automatically
// performs the migrations for those objects.
func gormDB(t testing.TB) *gorm.DB {
	vschema, err := dbmodel.SQL.ReadFile("sql/sqlite/versions.sql")
	if err != nil {
		t.Fatalf("error loading database schema: %s", err)
	}
	schema, err := dbmodel.SQL.ReadFile("sql/sqlite/0_0.sql")
	if err != nil {
		t.Fatalf("error loading database schema: %s", err)
	}
	db := jimmtest.MemoryDB(t, nil)
	if err := db.Exec(string(vschema)).Error; err != nil {
		t.Fatalf("error perform migrations on test database: %s", err)
	}
	if err := db.Exec(string(schema)).Error; err != nil {
		t.Fatalf("error perform migrations on test database: %s", err)
	}
	return db
}
