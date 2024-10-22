// Copyright 2024 Canonical.
package logger

import (
	"context"
	"fmt"

	"github.com/juju/zaputil/zapctx"
	"github.com/mattn/go-colorable"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SetupLogger sets up the default logger.
// The local logger is a colorized plain text logger.
// The production logger is a JSON structured logger.
func SetupLogger(ctx context.Context, logLevel string, logLocal bool) {
	pLogLevel, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		fmt.Printf("ERROR: log level '%s' cannot be parsed, defaulting to info\n", logLevel)
		pLogLevel = zap.InfoLevel
	}
	if logLocal {
		devConfig := zap.NewDevelopmentEncoderConfig()
		devConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		developLogger := zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(devConfig),
			zapcore.AddSync(colorable.NewColorableStdout()),
			pLogLevel,
		))
		zapctx.Default = developLogger
	} else {
		prodConfig := zap.NewProductionConfig()
		prodConfig.Level = zap.NewAtomicLevelAt(pLogLevel)
		proLogger := zap.Must(prodConfig.Build()) // this panic is an error is encountered during Build
		zapctx.Default = proLogger
	}
}
