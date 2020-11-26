// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/jackc/pgx/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestPostgres(t *testing.T) {
	c := qt.New(t)

	qtsuite.Run(c, &postgresSuite{})
}

type postgresSuite struct {
	dbSuite
}

func (s *postgresSuite) Init(c *qt.C) {
	dsn := os.Getenv("JIMM_TEST_PGXDSN")
	if dsn == "" {
		c.Skip("postgresql not configured")
	}

	connCfg, err := pgx.ParseConfig(dsn)
	c.Assert(err, qt.IsNil)

	// Every test runs in its own database.
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, connCfg)
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() { conn.Close(context.Background()) })

	var rnd [4]byte
	_, err = rand.Read(rnd[:])
	c.Assert(err, qt.IsNil)
	dbname := fmt.Sprintf("jimm_test_%s_%x", time.Now().Format("20060102"), rnd)
	_, err = conn.Exec(ctx, "CREATE DATABASE "+dbname)
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		_, err := conn.Exec(ctx, "DROP DATABASE "+dbname)
		if err != nil {
			c.Logf("cannot remove database %s: %s", dbname, err)
		}
	})

	testCfg := *connCfg
	testCfg.Database = dbname

	cfg := gorm.Config{
		Logger: jimmtest.NewGormLogger(c),
	}
	db, err := gorm.Open(postgres.Open(testCfg.ConnString()), &cfg)
	c.Assert(err, qt.IsNil)
	s.dbSuite.Database.DB = db
}
