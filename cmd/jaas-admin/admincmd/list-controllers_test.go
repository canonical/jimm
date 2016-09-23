// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	gc "gopkg.in/check.v1"
)

type listControllersSuite struct {
	commonSuite
}

var _ = gc.Suite(&listControllersSuite{})

var listControllersErrorTests = []struct {
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

func (s *listControllersSuite) TestError(c *gc.C) {
	for i, test := range listControllersErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "list-controllers", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}

func (s *listControllersSuite) TestSuccess(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// Add a couple of controllers.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "add-controller", "bob/bar")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "list-controllers")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/bar\nbob/foo\n")
}
