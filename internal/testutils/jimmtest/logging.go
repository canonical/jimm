// Copyright 2024 Canonical.
package jimmtest

import (
	"context"

	qt "github.com/frankban/quicktest"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/logger"
)

// SetupTestLogger adds a logger to the context
// that logs to the tests log output, so that
// it is visible in test failures.
func SetupTestLogger(c *qt.C) context.Context {
	testLogger := logger.NewTestLogger(c)
	return zapctx.WithLogger(c.Context(), testLogger)
}
