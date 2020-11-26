// Copyright 2020 Canonical Ltd.

package db_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestSQLite(t *testing.T) {
	c := qt.New(t)

	qtsuite.Run(c, &sqliteSuite{})
}

type sqliteSuite struct {
	dbSuite
}

func (s *sqliteSuite) Init(c *qt.C) {
	cfg := gorm.Config{
		Logger: jimmtest.NewGormLogger(c),
	}
	db, err := gorm.Open(sqlite.Open("file::memory:"), &cfg)
	c.Assert(err, qt.IsNil)
	// Enable foreign key constraints in SQLite, which are disabled by
	// default. This makes the encoded foreign key constraints behave as
	// expected.
	err = db.Exec("PRAGMA foreign_keys=ON").Error
	c.Assert(err, qt.IsNil)
	s.dbSuite.Database.DB = db
}
