// Copyright 2025 Canonical.

package db_test

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type advisoryLocksSuite struct {
	Database *db.Database
}

func (s *advisoryLocksSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	s.Database = db
}

func (s *advisoryLocksSuite) TestAdvisory_LockAndUnlock(c *qt.C) {
	ctx := c.Context()

	sqldb, err := s.Database.DB.DB()
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to get SQL DB connection."))
	sqlconn, err := sqldb.Conn(ctx)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to get SQL connection."))
	gdb2, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlconn,
	}))
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to open second GORM DB connection."))

	db2 := &db.Database{
		DB: gdb2,
	}

	// Acquire lock in db session 1.
	err = s.Database.LockBootstrap(ctx)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock."))

	// Attempt to acquire lock in session 2, should fail.
	err = db2.LockBootstrap(ctx)
	c.Assert(
		err,
		qt.ErrorMatches,
		"lock is already held",
		qt.Commentf("Expected lock acquisition to fail in second session."),
	)

	// Now unlock from session 1 and attempt to acquire in session 2 again.
	err = s.Database.UnlockBootstrap(ctx)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to release lock."))

	err = db2.LockBootstrap(ctx)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock in second session."))
}

func TestAdvisoryLocks(t *testing.T) {
	qtsuite.Run(qt.New(t), &advisoryLocksSuite{})
}
