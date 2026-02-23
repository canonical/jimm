// Copyright 2024 Canonical.

package logger

import (
	qt "github.com/frankban/quicktest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewGoCheckLogger create a logger to be used by the gocheck test library.
// The logs are shown only when the test fails.
func NewGoCheckLogger(c *qt.C) *zap.Logger {
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
	c *qt.C
}

func (w gocheckZapWriter) Write(buf []byte) (int, error) {
	w.c.Logf("%s", string(buf))
	return len(buf), nil
}

func (w gocheckZapWriter) Sync() error {
	return nil
}
