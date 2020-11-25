// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// dbSuite contains a suite of database tests that are run against
// different database engines.
type dbSuite struct {
	Database db.Database
}

func (s *dbSuite) TestMigrate(c *qt.C) {
	c.Parallel()

	// Migrate from an empty database should work.
	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	// Attempting to migrate to the version that is already there should
	// also work.
	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)
}

func TestMigrateUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var database db.Database
	err := database.Migrate(context.Background(), false)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}
