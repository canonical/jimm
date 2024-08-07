package cmd_test

import (
	"github.com/canonical/jimm/v3/internal/cmdtest"
	gc "gopkg.in/check.v1"
)

type cmdTestSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&cmdTestSuite{})
