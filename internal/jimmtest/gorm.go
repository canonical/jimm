// Copyright 2020 Canonical Ltd.

// Package jimmtest contains useful helpers for testing JIMM.
package jimmtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const unsafeCharsPattern = "[ .:;`'\"|<>~/\\?!@#$%^&*()[\\]{}=+-]"
const defaultDSN = "postgresql://jimm:jimm@127.0.0.1:5432/jimm"

// A Tester is the test interface required by this package.
type Tester interface {
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
	Name() string
}

// A gormLogger is a gorm.Logger that is used in tests. It logs everything
// to the test.
type gormLogger struct {
	t     Tester
	level logger.LogLevel
}

// NewGormLogger returns a gorm logger.Interface that can be used in a test
// All output is logged to the test.
func NewGormLogger(t Tester, l logger.LogLevel) logger.Interface {
	return gormLogger{t: t, level: l}
}

func (l gormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

func (l gormLogger) Info(_ context.Context, fmt string, args ...interface{}) {
	if l.level >= logger.Info {
		l.t.Logf(fmt, args...)
	}
}

func (l gormLogger) Warn(_ context.Context, fmt string, args ...interface{}) {
	if l.level >= logger.Warn {
		l.t.Logf(fmt, args...)
	}
}

func (l gormLogger) Error(_ context.Context, fmt string, args ...interface{}) {
	if l.level >= logger.Error {
		l.t.Logf(fmt, args...)
	}
}

func (l gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	errS := "<nil>"
	if err != nil {
		errS = fmt.Sprintf("%q", err.Error())
	}
	l.Info(ctx, "sql:%q rows:%d, error:%s, duration:%0.3fms", sql, rows, errS, float64(time.Since(begin).Microseconds())/10e3)
}

var _ logger.Interface = gormLogger{}

// MemoryDB returns a PostgreSQL database instance for tests.
func MemoryDB(t Tester, nowFunc func() time.Time) *gorm.DB {
	_, present := os.LookupEnv("TERSE")
	logLevel := logger.Info
	if present {
		logLevel = logger.Warn
	}
	cfg := gorm.Config{
		Logger:  NewGormLogger(t, logLevel),
		NowFunc: nowFunc,
	}

	re, _ := regexp.Compile("[ .:;`'\"|<>~/\\?!@#$%^&*()[\\]{}=+-]")
	schemaName := strings.ToLower("test_" + re.ReplaceAllString(t.Name(), "_"))

	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
	}

	gdb, err := gorm.Open(postgres.Open(dsn), &cfg)
	if err != nil {
		t.Fatalf("error opening database: %s", err)
	}

	createSchemaCommand := fmt.Sprintf(`
		DROP SCHEMA IF EXISTS "%[1]s";
		CREATE SCHEMA "%[1]s";
		SET search_path TO "%[1]s"`, // Make it as the default schema.
		schemaName,
	)
	if err := gdb.Exec(createSchemaCommand).Error; err != nil {
		t.Fatalf("error creating schema (%s): %s", schemaName, err)
	}

	return gdb
}

// CreateNewTestDatabase creates an empty Postgres database and returns the DSN.
func CreateEmptyDatabase(t Tester) string {
	re, _ := regexp.Compile(unsafeCharsPattern)
	dbName := strings.ToLower("jimm_test_" + re.ReplaceAllString(t.Name(), "_"))

	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
	}

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("error parsing DSN as a URI: %s", err)
	}

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("error opening database: %s", err)
	}

	dropDatabaseCommand := fmt.Sprintf(`DROP DATABASE IF EXISTS "%[1]s"`, dbName)
	if err := gdb.Exec(dropDatabaseCommand).Error; err != nil {
		t.Fatalf("error dropping exisiting database (%s): %s", dbName, err)
	}

	createDatabaseCommand := fmt.Sprintf(`CREATE DATABASE "%[1]s"`, dbName)
	if err := gdb.Exec(createDatabaseCommand).Error; err != nil {
		t.Fatalf("error creating database (%s): %s", dbName, err)
	}

	u.Path = dbName
	return u.String()
}
