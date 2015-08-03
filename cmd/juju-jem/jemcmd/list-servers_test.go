// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	gc "gopkg.in/check.v1"
)

type listServersSuite struct {
	commonSuite
}

var _ = gc.Suite(&listServersSuite{})

func (s *listServersSuite) TestChangePerm(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// Add a couple of state servers.
	stdout, stderr, code := run(c, c.MkDir(), "add-server", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "add-server", "bob/bar")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "list-servers")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/bar\nbob/foo\n")
}

var listServerErrorTests = []struct {
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

func (s *listServersSuite) TestError(c *gc.C) {
	for i, test := range listServerErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "list-servers", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
