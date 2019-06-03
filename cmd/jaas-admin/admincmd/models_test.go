// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	gc "gopkg.in/check.v1"
)

type modelsSuite struct {
	commonSuite
}

var _ = gc.Suite(&modelsSuite{})

func (s *modelsSuite) TestModels(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and some models.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addModel(c, "bob/foo", "bob/foo", "cred1")
	s.addModel(c, "bob/foo-1", "bob/foo", "cred1")
	s.addModel(c, "bob/foo-2", "bob/foo", "cred1")

	stdout, stderr, code = run(c, c.MkDir(), "models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\nbob/foo-1\nbob/foo-2\n")

	// Test the list-models alias works.
	stdout, stderr, code = run(c, c.MkDir(), "list-models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\nbob/foo-1\nbob/foo-2\n")
}

func (s *modelsSuite) TestAllModels(c *gc.C) {
	s.idmSrv.AddUser("alice", adminUser)
	s.idmSrv.SetDefaultUser("alice")

	// First add a controller and some models.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "alice/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addModel(c, "alice/bar", "alice/foo", "cred1")

	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")
	s.addModel(c, "bob/bar", "alice/foo", "cred1")

	s.idmSrv.SetDefaultUser("alice")
	stdout, stderr, code = run(c, c.MkDir(), "models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "alice/bar\n")

	stdout, stderr, code = run(c, c.MkDir(), "models", "--all")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "alice/bar\nbob/bar\n")

	// Test the list-models alias works.
	stdout, stderr, code = run(c, c.MkDir(), "list-models", "--all")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "alice/bar\nbob/bar\n")
}

var modelsErrorTests = []struct {
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

func (s *modelsSuite) TestError(c *gc.C) {
	for i, test := range modelsErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "models", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
