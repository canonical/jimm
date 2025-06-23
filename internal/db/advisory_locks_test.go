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

type advisoryLocksSuite struct{}

func (s *advisoryLocksSuite) TestAdvisory_LockAndUnlock(c *qt.C) {
	ctx := c.Context()

	gdb1, dbName := jimmtest.PostgresDBWithDbName(c, time.Now)
	dsn, _, err := jimmtest.GetTestDBDSN()
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to get test DB DSN"))

	dsn.Path = dbName
	gdb2, err := gorm.Open(postgres.Open(dsn.String()))
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to open second DB connection"))

	db1 := &db.Database{
		DB: gdb1,
	}

	db2 := &db.Database{
		DB: gdb2,
	}

	// Acquire lock in db session 1.
	err = db1.LockAdvisory(ctx, db.ControllerBootstrapLock)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock"))

	// Attempt to acquire lock in session 2, should fail.
	err = db2.LockAdvisory(ctx, db.ControllerBootstrapLock)
	c.Assert(
		err,
		qt.ErrorMatches,
		"lock is already held",
		qt.Commentf("Expected lock acquisition to fail in second session"),
	)

	// Now unlock from session 1 and attempt to acquire in session 2 again.
	err = db1.UnlockAdvisory(ctx, db.ControllerBootstrapLock)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to release lock"))

	err = db2.LockAdvisory(ctx, db.ControllerBootstrapLock)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock in second session"))
}

func TestAdvisoryLocks(t *testing.T) {
	qtsuite.Run(qt.New(t), &advisoryLocksSuite{})
}
