// Copyright 2025 Canonical.

package jujucommands_test

import (
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

	"github.com/canonical/jimm/v3/internal/jujucommands"
)

type jujucommandsSuite struct{}

func (s *jujucommandsSuite) TestRunCmdWithOutputRetriever(c *qt.C) {
	testCtx := c.Context()
	dir := c.TempDir()
	c.Patch(jujucommands.CmdPrefix, "echo")
	outputCh, err := jujucommands.RunJujuCmd(testCtx, []string{"i am an output"}, dir)
	c.Assert(err, qt.IsNil)

	// Use a builder to collect streamed output & test entire string.
	var b strings.Builder

	for out := range outputCh {
		c.Assert(out.Err, qt.IsNil)
		b.WriteString(out.Line)
	}

	expected := `i am an output`

	c.Assert(b.String(), qt.Equals, expected)
}

func (s *jujucommandsSuite) TestRunCmdWithOutputRetriever_Error(c *qt.C) {
	testCtx := c.Context()
	dir := c.TempDir()
	c.Patch(jujucommands.CmdPrefix, "ls")
	outputCh, err := jujucommands.RunJujuCmd(testCtx, []string{"-idontexist"}, dir)
	c.Assert(err, qt.IsNil)

	// The assertion for this test works such that we know we're going to receive 2 lines exactly.
	// The first line is a human readable error message, which we'll simply display to the user.
	// And contains no populated error field in out OutputLine struct.
	//
	// The second line is an actual error message from the command including the exit code.
	// It contains no line and just an error field in the OutputLine struct.
	var outputErr error
	var outputLineJustBeforeTheError string

	for out := range outputCh {
		if out.Err != nil {
			outputErr = out.Err
			continue
		}
		outputLineJustBeforeTheError = out.Line
	}

	c.Assert(outputLineJustBeforeTheError, qt.Equals, "Try 'ls --help' for more information.")
	c.Assert(outputErr, qt.ErrorMatches, "exit status 2")
}

func (s *jujucommandsSuite) TestEnvironmentIsCorrectlySet(c *qt.C) {
	testCtx := c.Context()
	c.Patch(jujucommands.CmdPrefix, "env")
	outputCh, err := jujucommands.RunJujuCmd(testCtx, []string{}, "testing-data-is-set")
	c.Assert(err, qt.IsNil)

	// Use a builder to collect streamed output & test entire string.
	var b strings.Builder

	for out := range outputCh {
		c.Assert(out.Err, qt.IsNil)
		b.WriteString(out.Line + "\n")
	}

	c.Assert(b.String(), qt.Equals, "JUJU_DATA=testing-data-is-set\n")
}

func TestJujucommandsSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &jujucommandsSuite{})
}
