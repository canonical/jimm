// Copyright 2024 Canonical.

package db_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

const (
	defaultDSN = "postgresql://jimm:jimm@127.0.0.1:5432/jimm"
)

func TestPostgres(t *testing.T) {
	c := qt.New(t)

	qtsuite.Run(c, &postgresSuite{})
}

type postgresSuite struct {
	dbSuite
}

func (s *postgresSuite) Init(c *qt.C) {
	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
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

	testCfg := connCfg.Copy()
	testCfg.Database = dbname

	sqlDB, err := sql.Open("pgx", stdlib.RegisterConnConfig(testCfg))
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			c.Logf("error closing database: %s", err)
		}
	})
	_, present := os.LookupEnv("TERSE")
	logLevel := logger.Info
	if present {
		logLevel = logger.Warn
	}
	cfg := gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC().Round(time.Millisecond) },
		Logger:  jimmtest.NewGormLogger(c, logLevel),
	}
	pCfg := postgres.Config{
		Conn: sqlDB,
	}
	gdb, err := gorm.Open(postgres.New(pCfg), &cfg)
	c.Assert(err, qt.IsNil)
	s.dbSuite.Database = &db.Database{DB: gdb}
}
