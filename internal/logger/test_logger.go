// Copyright 2024 Canonical.

package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Logf(format string, args ...any)
}

// NewTestLogger create a logger to be used by tests.
// The logs are shown only when the test fails.
func NewTestLogger(l Logger) *zap.Logger {
	output := testZapWriter{l}

	devConfig := zap.NewDevelopmentEncoderConfig()
	devConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	devConfig.EncodeTime = shortTimeEncoder

	return zap.New(
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(devConfig),
			output,
			zap.DebugLevel,
		),
	)
}

type testZapWriter struct {
	l Logger
}

func (w testZapWriter) Write(buf []byte) (int, error) {
	w.l.Logf("%s", string(buf))
	return len(buf), nil
}

func (w testZapWriter) Sync() error {
	return nil
}
