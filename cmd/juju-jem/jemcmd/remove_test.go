// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type removeSuite struct {
	commonSuite
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) TestRemoveModel(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addEnv(c, "bob/foo-1", "bob/foo")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "bob/foo-1")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "list")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\n")
}

func (s *removeSuite) TestRemoveServer(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Add a second controller, that won't be deleted.
	stdout, stderr, code = run(c, c.MkDir(), "add-controller", "bob/bar")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addEnv(c, "bob/foo-1", "bob/foo")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "--controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "list-controllers")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/bar\n")

	stdout, stderr, code = run(c, c.MkDir(), "list")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/bar\n")
}

func (s *removeSuite) TestRemoveTemplate(c *gc.C) {
	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")
	client := s.jemClient("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--controller", "bob/foo", "bob/mytemplate", "controller=true", "apt-mirror=0.1.2.3")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	_, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)

	stdout, stderr, code = run(c, c.MkDir(), "remove", "--template", "bob/mytemplate")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	_, err = client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *removeSuite) TestRemoveMultipleModels(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addEnv(c, "bob/foo-1", "bob/foo")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "bob/foo", "bob/foo-1")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `cannot remove bob/foo: DELETE http://.*/v2/model/bob/foo: cannot remove model "bob/foo" because it is a controller`+"\nERROR not all models removed successfully\n")

	stdout, stderr, code = run(c, c.MkDir(), "list")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "bob/foo\n")
}

func (s *removeSuite) TestRemoveVerbose(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller and an model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.addEnv(c, "bob/foo-1", "bob/foo")

	stdout, stderr, code = run(c, c.MkDir(), "remove", "--verbose", "bob/foo-1")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "removing bob/foo-1\n")

	stdout, stderr, code = run(c, c.MkDir(), "list")
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
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
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
