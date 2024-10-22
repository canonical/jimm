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
	defer func() {
		r := recover()
		if r != nil {
			t.Errorf("The code panic")
		}
	}()

	tests := []struct {
		logLevel string
		logLocal bool
	}{
		{
			logLevel: "info",
			logLocal: true,
		},
		{
			logLevel: "info",
			logLocal: false,
		},
		{
			logLevel: "",
			logLocal: false,
		},
		{
			logLevel: "error",
			logLocal: false,
		},
		{
			logLevel: "error",
			logLocal: true,
		},
		{
			logLevel: "not_exisiting_level",
			logLocal: true,
		},
	}

	for _, t := range tests {
		logger.SetupLogger(ctx, t.logLevel, t.logLocal)
		zapctx.Info(ctx, "test log")
	}
}
