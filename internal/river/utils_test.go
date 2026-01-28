// Copyright 2026 Canonical.

package river

import (
	"time"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	qt "github.com/frankban/quicktest"
)

func setupTestDB(c *qt.C) *db.Database {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(c.Context())
	c.Assert(err, qt.IsNil)
	err = MigrateRiver(c.Context(), db)
	c.Assert(err, qt.IsNil)
	return db
}
