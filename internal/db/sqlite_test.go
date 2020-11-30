// Copyright 2020 Canonical Ltd.

package db_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

	"github.com/CanonicalLtd/jimm/internal/db"
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
	s.dbSuite.Database = &db.Database{DB: jimmtest.MemoryDB(c, nil)}
}
