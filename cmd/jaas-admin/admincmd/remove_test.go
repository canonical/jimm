// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	gc "gopkg.in/check.v1"
)

type removeSuite struct {
	commonSuite
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) TestRemoveModel(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.addModel(c, "bob/foo", "bob/foo", "cred1")

	s.addModel(c, "bob/foo-1", "bob/foo", "cred1")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "bob/foo-1")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\n")
}

func (s *removeSuite) TestRemoveController(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.addModel(c, "bob/foo", "bob/foo", "cred1")

	// Add a second controller, that won't be deleted.
	stdout, stderr, code = run(c, c.MkDir(), "add-controller", "bob/bar")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.addModel(c, "bob/bar", "bob/bar", "cred1")

	s.addModel(c, "bob/foo-1", "bob/foo", "cred1")

	// Without the --force flag, we'll be forbidden because the controller
	// is live.
	stdout, stderr, code = run(c, c.MkDir(), "remove", "--controller", "bob/foo")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `cannot remove bob/foo: cannot delete controller while it is still alive\n`)

	// We can use the --force flag to remove it.
	stdout, stderr, code = run(c, c.MkDir(), "remove", "--force", "--controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "list-controllers")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/bar\n")

	stdout, stderr, code = run(c, c.MkDir(), "models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/bar\n")
}

func (s *removeSuite) TestRemoveMultipleModels(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addModel(c, "bob/foo-1", "bob/foo", "cred1")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "bob/foo", "bob/foo-1")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `cannot remove bob/foo: model "bob/foo" not found`+"\n")

	stdout, stderr, code = run(c, c.MkDir(), "models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "")
}

func (s *removeSuite) TestRemoveVerbose(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.addModel(c, "bob/foo", "bob/foo", "cred1")

	s.addModel(c, "bob/foo-1", "bob/foo", "cred1")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "--verbose", "bob/foo-1")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "removing bob/foo-1\n")

	stdout, stderr, code = run(c, c.MkDir(), "models")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\n")
}

var removeErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "invalid path",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": need <user>/<name>`,
	expectCode:   2,
}}

func (s *removeSuite) TestError(c *gc.C) {
	for i, test := range removeErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "remove", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
