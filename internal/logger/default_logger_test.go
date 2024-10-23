// Copyright 2024 Canonical.
package logger_test

import (
	"context"
	"testing"

	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/logger"
)

// TestLogSetup tests log setup with different configurations and checks for panic.
func TestLogSetup(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		description string
		logLevel    string
		devMode     bool
	}{
		{

			description: "info and dev model",
			logLevel:    "info",
			devMode:     true,
		},
		{
			description: "info and prod model",
			logLevel:    "info",
			devMode:     false,
		},
		{
			description: "default log mode",
			logLevel:    "",
			devMode:     false,
		},
		{
			description: "error and dev model",
			logLevel:    "error",
			devMode:     true,
		},
		{
			description: "error and prod model",
			logLevel:    "error",
			devMode:     false,
		},
		{
			description: "not existing level",
			logLevel:    "not_exisiting_level",
			devMode:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer func() {
				r := recover()
				if r != nil {
					t.Errorf("The code panic")
				}
			}()
			logger.SetupLogger(ctx, tt.logLevel, tt.devMode)
			zapctx.Info(ctx, "test log")
		})
	}
}
