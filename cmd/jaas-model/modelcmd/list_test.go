// Copyright 2015 Canonical Ltd.

package modelcmd_test

import (
	gc "gopkg.in/check.v1"
)

type listSuite struct {
	commonSuite
}

var _ = gc.Suite(&listSuite{})

func (s *listSuite) TestList(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and some models.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addEnv(c, "bob/foo-1", "bob/foo")
	s.addEnv(c, "bob/foo-2", "bob/foo")

	stdout, stderr, code = run(c, c.MkDir(), "list")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\nbob/foo-1\nbob/foo-2\n")
}

var listErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too many arguments",
	args:         []string{"bad"},
	expectStderr: "got 1 arguments, want none",
	expectCode:   2,
}}

func (s *listSuite) TestError(c *gc.C) {
	for i, test := range listErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "list", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
