// Copyright 2016 Canonical Ltd.

// Package zapctx provides support for associating zap loggers
// (see github.com/uber-go/zap) with contexts.
package zapctx

import (
	"os"

	"github.com/uber-go/zap"
	"golang.org/x/net/context"
)

// Default holds the logger returned by Logger when
// there is no logger in the context.
var Default = zap.New(zap.NewJSONEncoder(), zap.Output(os.Stderr))

// loggerKey holds the context key used for loggers.
type loggerKey struct{}

// WithLogger returns a new context derived from ctx that
// is associated with the given logger.
func WithLogger(ctx context.Context, logger zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// Logger returns the logger associated with the given
// context. If there is no logger, it will return Default.
func Logger(ctx context.Context) zap.Logger {
	if logger, _ := ctx.Value(loggerKey{}).(zap.Logger); logger != nil {
		return logger
	}
	return Default
}
