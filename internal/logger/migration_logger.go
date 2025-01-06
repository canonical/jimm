// Copyright 2024 Canonical.

package logger

import (
	"fmt"

	"go.uber.org/zap"
)

// MigrationLogger provides a logger for use with DB migrations.
type MigrationLogger struct {
	Logger    *zap.Logger
	IsVerbose bool
}

// Printf implements the Printf function of the migrate.Logger interface.
func (l MigrationLogger) Printf(format string, v ...interface{}) {
	line := fmt.Sprintf(format, v...)
	// Remove unneeded new lines since the zap logger adds them.
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	l.Logger.Info(line)
}

// Verbose implements the Verbose function of the migrate.Logger interface.
func (l MigrationLogger) Verbose() bool {
	return l.IsVerbose
}
