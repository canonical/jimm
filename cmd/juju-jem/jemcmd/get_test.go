// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"github.com/juju/juju/juju"
	gc "gopkg.in/check.v1"
)

type getSuite struct {
	commonSuite
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) TestGet(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a state server. This also adds an environment that we can
	// get for our test.
	stdout, stderr, code := run(c, c.MkDir(), "add-server", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	apiInfo := s.APIInfo(c)
	stdout, stderr, code = run(c, c.MkDir(),
		"get",
		"--password", apiInfo.Password,
		"bob/foo",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that we can attach to the new environment
	// through the usual juju connection mechanism.
	client, err := juju.NewAPIClientFromName("foo")
	c.Assert(err, gc.IsNil)
	client.Close()
}

var getErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too few arguments",
	args:         []string{},
	expectStderr: "got 0 arguments, want 1",
	expectCode:   2,
}, {
	about:        "too many arguments",
	args:         []string{"a", "b", "c"},
	expectStderr: "got 3 arguments, want 1",
	expectCode:   2,
}, {
	about:        "only one part in environ id",
	args:         []string{"a"},
	expectStderr: `invalid JEM name "a" \(needs to be <user>/<name>\)`,
	expectCode:   2,
}}

func (s *getSuite) TestGetError(c *gc.C) {
	for i, test := range getErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "get", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
