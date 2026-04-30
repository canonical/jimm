// Copyright 2024 Canonical.
package logger

import (
	"os"
	"strconv"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm/logger"
)

// A Tester is the test interface required by gorm for the logger.
type Tester interface {
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
	Name() string
	Cleanup(f func())
}

// A gormLogger is a gorm.Logger that is used in tests. It logs everything
// to the test. The logs are flushed only if the test fails.
type gormLogger struct {
	GormLogger
	t Tester
}

// NewGormLogger returns a gorm logger.Interface that can be used in a test
// All output is logged to the test.
func NewGormTestLogger(t Tester) logger.Interface {
	output := gormTesterZapWriter{t}

	devConfig := zap.NewDevelopmentEncoderConfig()
	devConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	devConfig.EncodeTime = shortTimeEncoder

	logger := zap.New(
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(devConfig),
			output,
			zap.WarnLevel,
		))
	zapctx.Default = logger
	logSQL, _ := strconv.ParseBool(os.Getenv("JIMM_TEST_LOG_SQL"))
	return &gormLogger{
		t: t,
		GormLogger: GormLogger{
			LogSQL: logSQL,
		},
	}
}

// gormTesterZapWriter is a zap writer wrapping the Tester interface.
// It is used to comply with the behaviour of flushing the logs only in case of failure.
type gormTesterZapWriter struct {
	t Tester
}

func (w gormTesterZapWriter) Write(buf []byte) (int, error) {
	w.t.Logf(string(buf))
	return len(buf), nil
}

func (w gormTesterZapWriter) Sync() error {
	return nil
}
