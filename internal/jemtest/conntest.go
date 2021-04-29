// Package jemtest provides test fixtures for testing JEM.
package jemtest

import (
	"github.com/google/go-cmp/cmp"
	corejujutesting "github.com/juju/juju/juju/testing"
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

// CmpEquals uses cmp.Diff (see http://godoc.org/github.com/google/go-cmp/cmp#Diff)
// to compare two values, passing opts to the comparer to enable custom
// comparison.
func CmpEquals(opts ...cmp.Option) gc.Checker {
	return &cmpEqualsChecker{
		CheckerInfo: &gc.CheckerInfo{
			Name:   "CmpEquals",
			Params: []string{"obtained", "expected"},
		},
		check: func(params []interface{}, names []string) (result bool, error string) {
			if diff := cmp.Diff(params[0], params[1], opts...); diff != "" {
				return false, diff
			}
			return true, ""
		},
	}
}

type cmpEqualsChecker struct {
	*gc.CheckerInfo
	check func(params []interface{}, names []string) (result bool, error string)
}

func (c *cmpEqualsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	return c.check(params, names)
}
