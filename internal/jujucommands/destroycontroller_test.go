// Copyright 2025 Canonical.

package jujucommands_test

import (
	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jujucommands"
)

func (s *jujucommandsSuite) TestDestroyControllerCmdParams_Validate(c *qt.C) {
	p := jujucommands.DestroyControllerCmdParams{}
	c.Assert(p.Validate(), qt.ErrorMatches, ".*controller name cannot be empty.*")

	p.ControllerName = "foo"
	c.Assert(p.Validate(), qt.IsNil)
}

func (s *jujucommandsSuite) TestDestroyControllerCmdParams_Args(c *qt.C) {
	params := jujucommands.DestroyControllerCmdParams{
		ControllerName: "my-controller",
	}
	expect := []string{
		"destroy-controller",
		"my-controller",
		"--no-prompt",
	}

	args := params.BuildCmdArgs()
	c.Assert(args, qt.DeepEquals, expect)
}
