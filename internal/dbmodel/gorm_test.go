// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gormDB creates a new *gorm.DB for use in tests. The newly created DB
// will be an in-memory SQLite database that logs to the given test with
// debug enabled. If any objects are specified the datbase automatically
// performs the migrations for those objects.
func gormDB(t testing.TB, objects ...interface{}) *gorm.DB {
	cfg := gorm.Config{
		Logger: testLogger{t},
	}
	db, err := gorm.Open(sqlite.Open("file::memory:"), &cfg)
	if err != nil {
		t.Fatalf("error creating test database: %s", err)
	}
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		t.Fatalf("error enabling foreign keys: %s", err)
	}
	err = db.AutoMigrate(objects...)
	if err != nil {
		t.Fatalf("error perform migrations on test database: %s", err)
	}
	return db
}

// A testLogger is a gorm.Logger that is used in tests. It logs everything
// to the test.
type testLogger struct {
	t testing.TB
}

func (l testLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

func (l testLogger) Info(_ context.Context, fmt string, args ...interface{}) {
	l.t.Logf(fmt, args...)
}

func (l testLogger) Warn(_ context.Context, fmt string, args ...interface{}) {
	l.t.Logf(fmt, args...)
}

func (l testLogger) Error(_ context.Context, fmt string, args ...interface{}) {
	l.t.Logf(fmt, args...)
}

func (l testLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	errS := "<nil>"
	if err != nil {
		errS = fmt.Sprintf("%q", err.Error())
	}
	l.Info(ctx, "sql:%q rows:%d, error:%s, duration:%0.3fms", sql, rows, errS, float64(time.Since(begin).Microseconds())/10e3)
}

var _ logger.Interface = testLogger{}
