// Copyright 2024 Canonical.

package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gc "gopkg.in/check.v1"
)

// NewGoCheckLogger create a logger to be used by the gocheck test library.
// The logs are shown only when the test fails.
func NewGoCheckLogger(c *gc.C) *zap.Logger {
	output := gocheckZapWriter{c}

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

type gocheckZapWriter struct {
	c *gc.C
}

func (w gocheckZapWriter) Write(buf []byte) (int, error) {
	w.c.Logf(string(buf))
	return len(buf), nil
}

func (w gocheckZapWriter) Sync() error {
	return nil
}
