// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

// gormDB creates a new *gorm.DB for use in tests. The newly created DB
// will be an in-memory SQLite database that logs to the given test with
// debug enabled. If any objects are specified the datbase automatically
// performs the migrations for those objects.
func gormDB(t testing.TB, objects ...interface{}) *gorm.DB {
	db := jimmtest.MemoryDB(t, nil)
	err := db.AutoMigrate(objects...)
	if err != nil {
		t.Fatalf("error perform migrations on test database: %s", err)
	}
	return db
}
