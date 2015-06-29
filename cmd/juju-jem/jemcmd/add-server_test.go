// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type addServerSuite struct {
	commonSuite
}

var _ = gc.Suite(&addServerSuite{})

func (s *addServerSuite) TestAddServer(c *gc.C) {
	s.username = "bob"
	_, err := s.jemClient.GetJES(&params.GetJES{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	stdout, stderr, code := run(c, c.MkDir(), "add-server", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	_, err = s.jemClient.GetJES(&params.GetJES{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)
}

var addServerErrorTests = []struct {
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
	about:        "only one part in server id",
	args:         []string{"a"},
	expectStderr: `invalid JEM environment name \(needs to be <user>/<name>\)`,
	expectCode:   2,
}, {
	about:        "too many parts in server id",
	args:         []string{"a/b/c"},
	expectStderr: `invalid JEM environment name \(needs to be <user>/<name>\)`,
	expectCode:   2,
}, {
	about:        "empty server id",
	args:         []string{""},
	expectStderr: `invalid JEM environment name \(needs to be <user>/<name>\)`,
	expectCode:   2,
}, {
	about:        "invalid name checked by server",
	args:         []string{"bad+name/foo"},
	expectStderr: `cannot add state server: cannot unmarshal parameters: cannot unmarshal into field: invalid user name "bad\+name"`,
	expectCode:   1,
}}

func (s *addServerSuite) TestAddServerError(c *gc.C) {
	for i, test := range addServerErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "add-server", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
