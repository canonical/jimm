// Copyright 2020 Canonical Ltd.

// Package jimmtest contains useful helpers for testing JIMM.
package jimmtest

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// A Tester is the test interface required by this package.
type Tester interface {
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
	Name() string
}

// A gormLogger is a gorm.Logger that is used in tests. It logs everything
// to the test.
type gormLogger struct {
	t Tester
}

// NewGormLogger returns a gorm logger.Interface that can be used in a test
// All output is logged to the test.
func NewGormLogger(t Tester) logger.Interface {
	return gormLogger{t: t}
}

func (l gormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

func (l gormLogger) Info(_ context.Context, fmt string, args ...interface{}) {
	l.t.Logf(fmt, args...)
}

func (l gormLogger) Warn(_ context.Context, fmt string, args ...interface{}) {
	l.t.Logf(fmt, args...)
}

func (l gormLogger) Error(_ context.Context, fmt string, args ...interface{}) {
	l.t.Logf(fmt, args...)
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

// MemoryDB returns an in-memory gorm.DB for use in tests. The underlying
// SQL database is an in-memory SQLite database.
func MemoryDB(t Tester, nowFunc func() time.Time) *gorm.DB {
	cfg := gorm.Config{
		Logger:  NewGormLogger(t),
		NowFunc: nowFunc,
	}
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &cfg)
	if err != nil {
		t.Fatalf("error opening database: %s", err)
	}
	// Enable foreign key constraints in SQLite, which are disabled by
	// default. This makes the encoded foreign key constraints behave as
	// expected.
	if err = gdb.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatalf("cannot enable foreign keys: %s", err)
	}
	return gdb
}
