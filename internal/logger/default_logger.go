// Copyright 2024 Canonical.

package logger

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/zaputil/zapctx"
	"github.com/mattn/go-colorable"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SetupLogger sets up the default logger.
// The local logger is a colorized plain text logger.
// The production logger is a JSON structured logger.
func SetupLogger(ctx context.Context, logLevel string, devMode bool) {
	pLogLevel, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		fmt.Printf("ERROR: log level %q cannot be parsed, defaulting to info\n", logLevel)
		pLogLevel = zap.InfoLevel
	}
	if devMode {
		devConfig := zap.NewDevelopmentEncoderConfig()
		devConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		devConfig.EncodeTime = shortTimeEncoder
		developLogger := zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(devConfig),
			zapcore.AddSync(colorable.NewColorableStdout()),
			pLogLevel,
		))
		zapctx.Default = developLogger
	} else {
		prodConfig := zap.NewProductionConfig()
		prodConfig.Level = zap.NewAtomicLevelAt(pLogLevel)
		proLogger := zap.Must(prodConfig.Build()) // this panics if an error is encountered during Build
		zapctx.Default = proLogger
	}
}

// shortTimeEncoder encodes time as 15:04:05.000
func shortTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("15:04:05.000"))
}
