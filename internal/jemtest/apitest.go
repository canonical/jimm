// Package jemtest provides test fixtures for testing JEM.
package jemtest

import (
	corejujutesting "github.com/juju/juju/juju/testing"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

// JujuConnSuite implements a variant of github.com/juju/juju/juju/testing.JujuConnSuite
// that uses zap for logging.
type JujuConnSuite struct {
	corejujutesting.JujuConnSuite
	LoggingSuite
}

func (s *JujuConnSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *JujuConnSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
}

func (s *JujuConnSuite) TearDownTest(c *gc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

// IsolatedMgoSuite implements a variant of github.com/juju/testing.IsolatedMgoSuite
// that uses zap for logging.
type IsolatedMgoSuite struct {
	jujutesting.IsolatedMgoSuite
	LoggingSuite
}

func (s *IsolatedMgoSuite) SetUpSuite(c *gc.C) {
	s.IsolatedMgoSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *IsolatedMgoSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.IsolatedMgoSuite.TearDownSuite(c)
}

func (s *IsolatedMgoSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
}

func (s *IsolatedMgoSuite) TearDownTest(c *gc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.IsolatedMgoSuite.TearDownTest(c)
}
