// Copyright 2024 Canonical.
package jimmtest

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/zaputil/zapctx"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/logger"
)

// init is a workaround to reset the logger coming from the loggo library.
// This is to prevent juju debug logs from cluttering our tests output.
func init() {
	ticker := time.NewTicker(100 * time.Millisecond)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				loggo.ResetLogging()
			}
		}
	}()
}

// LoggingSuite is a replacement for github.com/juju/testing.LoggingSuite
// zap logging but also replaces the global loggo logger.
// When used with juju/testing.LoggingSuite, it should
// be set up after that.
type LoggingSuite struct{}

func (s *LoggingSuite) SetUpSuite(c *gc.C) {
	s.setUp(c)
}

func (s *LoggingSuite) TearDownSuite(c *gc.C) {
}

func (s *LoggingSuite) SetUpTest(c *gc.C) {
	s.setUp(c)
}

func (s *LoggingSuite) TearDownTest(c *gc.C) {
}

func (s *LoggingSuite) setUp(c *gc.C) {
	goCheckLogger := logger.NewGoCheckLogger(c)
	zapctx.Default = goCheckLogger
	loggo.ResetLogging()
}
