// Copyright 2024 Canonical.

// Package logger contains logger adapters for various services.
package logger

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm/logger"
)

// GormLogger is an implementation of gorm's logger.Interface that logs
// using zapctx.
type GormLogger struct {
	LogSQL bool
}

// LogMode implements the LogMode function of logger.Interface. This always
// returns an identical implementation, the log level is handled by zap.
func (GormLogger) LogMode(logger.LogLevel) logger.Interface {
	return GormLogger{}
}

// Error implements logger.Interface, it logs at ERROR level.
func (GormLogger) Error(ctx context.Context, f string, args ...interface{}) {
	zapctx.Error(ctx, fmt.Sprintf(f, args...))
}

// Warn implements logger.Interface, it logs at WARN level.
func (GormLogger) Warn(ctx context.Context, f string, args ...interface{}) {
	zapctx.Warn(ctx, fmt.Sprintf(f, args...))
}

// Info implements logger.Interface, it logs at INFO level.
func (GormLogger) Info(ctx context.Context, f string, args ...interface{}) {
	zapctx.Info(ctx, fmt.Sprintf(f, args...))
}

// Trace implements logger.Interface, it logs at DEBUG level.
func (g GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	// Avoid logging SQL to prevent leaking secrets
	if !g.LogSQL {
		return
	}
	ce := zapctx.Logger(ctx).Check(zapcore.DebugLevel, "TRACE")
	if ce == nil {
		return
	}
	fields := make([]zapcore.Field, 3, 4)
	fields[0] = zap.Stringer("time", time.Since(begin))
	sql, rows := fc()
	fields[1] = zap.String("sql", sql)
	fields[2] = zap.Int64("rows", rows)
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	ce.Write(fields...)
}

var _ logger.Interface = GormLogger{}
