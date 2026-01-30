// Copyright 2026 Canonical.

package river

import (
	"database/sql"
	"time"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river/rivertype"
)

func setupTestDB(c *qt.C) (*db.Database, *sql.DB) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(c.Context())
	c.Assert(err, qt.IsNil)
	err = MigrateRiver(c.Context(), db)
	c.Assert(err, qt.IsNil)
	sqlDB, err := db.SqlDB()
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		c.Check(sqlDB.Close(), qt.IsNil)
	})
	return db, sqlDB
}

type testRetryPolicy struct{}

// NextRetry implements the [river.ClientRetryPolicy] interface.
// It ensures retries happen quickly during tests.
func (p *testRetryPolicy) NextRetry(job *rivertype.JobRow) time.Time {
	return time.Now().Add(1 * time.Millisecond)
}
