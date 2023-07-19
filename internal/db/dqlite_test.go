// Copyright 2020 Canonical Ltd.

// go-dqlite only works properly if the go-sqlite3 package is built with
// the libsqlite3 tag.
// +build libsqlite3
//go:build libsqlite3

package db_test

import (
	"context"
	"testing"

	"github.com/canonical/go-dqlite/app"
	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimmtest"
)

func TestDQLite(t *testing.T) {
	c := qt.New(t)
	app, err := app.New(c.Mkdir())
	c.Assert(err, qt.IsNil)
	err = app.Ready(context.Background())
	c.Assert(err, qt.IsNil)

	qtsuite.Run(c, &dqliteSuite{app: app})
}

type dqliteSuite struct {
	dbSuite
	app *app.App
}

func (s *dqliteSuite) Init(c *qt.C) {
	cfg := gorm.Config{
		Logger: jimmtest.NewGormLogger(c),
	}
	sqldb, err := s.app.Open(context.Background(), c.Name())
	c.Assert(err, qt.IsNil)

	dialector := sqlite.Dialector{
		DriverName: "dqlite",
		Conn:       sqldb,
	}

	gdb, err := gorm.Open(&dialector, &cfg)
	c.Assert(err, qt.IsNil)

	// Enable foreign key constraints in SQLite, which are disabled by
	// default. This makes the encoded foreign key constraints behave as
	// expected.
	err = gdb.Exec("PRAGMA foreign_keys=ON").Error
	c.Assert(err, qt.IsNil)
	s.dbSuite.Database = &db.Database{DB: gdb}
}
