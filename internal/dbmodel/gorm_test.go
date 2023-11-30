// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimmtest"
)

// gormDB creates a new *gorm.DB for use in tests. The newly created DB
// will be a Postgres database that logs to the given test with debug enabled.
// If any objects are specified the database automatically performs the
// migrations for those objects.
func gormDB(t testing.TB) *gorm.DB {
	database := db.Database{DB: jimmtest.MemoryDB(t, nil)}
	database.Migrate(context.Background(), false)
	return database.DB
}
