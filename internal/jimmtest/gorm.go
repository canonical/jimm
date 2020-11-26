// Copyright 2020 Canonical Ltd.

// Package jimmtest contains useful helpers for testing JIMM.
package jimmtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm/logger"
)

// A gormLogger is a gorm.Logger that is used in tests. It logs everything
// to the test.
type gormLogger struct {
	t testing.TB
}

// NewGormLogger returns a gorm logger.Interface that can be used in a test
// All output is logged to the test.
func NewGormLogger(t testing.TB) logger.Interface {
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
