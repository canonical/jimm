// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// dbSuite contains a suite of database tests that are run against
// different database engines.
type dbSuite struct {
	Database *db.Database
}

func (s *dbSuite) TestMigrate(c *qt.C) {
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

func TestTransactionUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var database db.Database
	err := database.Transaction(func(d *db.Database) error {
		return errors.E("unexpected function call")
	})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestTransaction(c *qt.C) {
	err := s.Database.Transaction(func(d *db.Database) error {
		return errors.E("unexpected function call")
	})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.Transaction(func(d *db.Database) error {
		c.Check(d, qt.Not(qt.Equals), s.Database)
		return d.GetUser(context.Background(), &dbmodel.Identity{Username: "bob@external"})
	})
	c.Assert(err, qt.IsNil)

	err = s.Database.Transaction(func(d *db.Database) error {
		return errors.E("test error")
	})
	c.Check(err, qt.ErrorMatches, `test error`)
}
