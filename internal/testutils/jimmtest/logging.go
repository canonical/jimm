// Copyright 2024 Canonical.
package jimmtest

import (
	qt "github.com/frankban/quicktest"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/logger"
)

// LoggingSuite is a replacement for github.com/juju/testing.LoggingSuite
// zap logging but also replaces the global loggo logger.
// When used with juju/testing.LoggingSuite, it should
// be set up after that.
func SetupTestLogger(c *qt.C) {
	goCheckLogger := logger.NewGoCheckLogger(c)
	zapctx.Default = goCheckLogger
}
