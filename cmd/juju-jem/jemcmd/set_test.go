// Copyright 2016 Canonical Ltd.

package jemcmd_test

import (
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/params"
)

type setSuite struct {
	commonSuite
}

var _ = gc.Suite(&setSuite{})

func (s *setSuite) TestSet(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check we don't have location attributes.
	jemclient := s.jemClient("bob")
	resp, err := jemclient.GetControllerLocation(&params.GetControllerLocation{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.DeepEquals, params.ControllerLocation{Location: map[string]string{}})

	// Set location attributes.
	stdout, stderr, code = run(c, c.MkDir(),
		"set",
		"bob/foo",
		"cloud=aws",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check the location attributes.
	resp, err = jemclient.GetControllerLocation(&params.GetControllerLocation{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.DeepEquals, params.ControllerLocation{Location: map[string]string{
		"cloud": "aws",
	}})

	//Add one more location attribute without loosing the cloud attribute.
	stdout, stderr, code = run(c, c.MkDir(),
		"set",
		"bob/foo",
		"region=us-east1",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check the location attributes.
	resp, err = jemclient.GetControllerLocation(&params.GetControllerLocation{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(resp, gc.DeepEquals, params.ControllerLocation{Location: map[string]string{
		"cloud":  "aws",
		"region": "us-east1",
	}})
}

var setErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too few arguments",
	args:         []string{},
	expectStderr: "got 0 arguments, want at least 2",
	expectCode:   2,
}, {
	about:        "only one part in model id",
	args:         []string{"a", "b"},
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
	expectCode:   2,
}, {
	about:        "invalid key/value pair",
	args:         []string{"bob/controller", "something"},
	expectStderr: `invalid set arguments: expected "key=value", got "something"`,
	expectCode:   2,
}}

func (s *setSuite) TestSetError(c *gc.C) {
	for i, test := range setErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "set", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
