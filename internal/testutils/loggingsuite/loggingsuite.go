// Copyright 2024 Canonical.
package loggingsuite

import (
	"context"
	"os"

	"github.com/juju/loggo"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/logger"
)

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
	logger.SetupLogger(context.Background(), "debug", true)
	loggo.ResetLogging()
	// Don't use the default writer for the test logging, which
	// means we can still get logging output from tests that
	// replace the default writer.
	err := loggo.RegisterWriter(loggo.DefaultWriterName, discardWriter{})
	c.Assert(err, gc.IsNil)
	err = loggo.RegisterWriter("loggingsuite", zaputil.NewLoggoWriter(zapctx.Default))
	c.Assert(err, gc.IsNil)
	level := "DEBUG"
	if envLevel := os.Getenv("TEST_LOGGING_CONFIG"); envLevel != "" {
		level = envLevel
	}
	err = loggo.ConfigureLoggers(level)
	c.Assert(err, gc.Equals, nil)
}

type discardWriter struct{}

func (discardWriter) Write(entry loggo.Entry) {
}
