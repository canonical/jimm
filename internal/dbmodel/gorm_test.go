// Copyright 2024 Canonical.

package dbmodel_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// gormDB creates a new *gorm.DB for use in tests. The newly created DB
// will be a Postgres database that logs to the given test with debug enabled.
// If any objects are specified the database automatically performs the
// migrations for those objects.
func gormDB(t testing.TB) *gorm.DB {
	database := db.Database{DB: jimmtest.PostgresDB(t, nil)}
	err := database.Migrate(context.Background(), false)
	if err != nil {
		t.Fail()
	}
	return database.DB
}
