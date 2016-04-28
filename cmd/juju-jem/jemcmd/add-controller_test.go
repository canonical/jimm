// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"fmt"

	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type addControllerSuite struct {
	commonSuite
}

var _ = gc.Suite(&addControllerSuite{})

var addControllerTests = []struct {
	about    string
	args     []string
	location map[string]string
}{{
	about:    "simple",
	args:     []string{},
	location: map[string]string{},
}, {
	about:    "add cloud",
	args:     []string{"cloud=aws"},
	location: map[string]string{"cloud": "aws"},
}, {
	about:    "add region",
	args:     []string{"region=somewhere"},
	location: map[string]string{"region": "somewhere"},
}, {
	about:    "add region and cloud",
	args:     []string{"region=somewhere", "cloud=aws"},
	location: map[string]string{"cloud": "aws", "region": "somewhere"},
}}

func (s *addControllerSuite) TestAddController(c *gc.C) {
	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")
	client := s.jemClient("bob")
	for i, test := range addControllerTests {
		c.Logf("test %d: %s", i, test.about)
		_, err := client.GetController(&params.GetController{
			EntityPath: params.EntityPath{
				User: "bob",
				Name: params.Name(fmt.Sprintf("foo-%v", i)),
			},
		})
		c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
		test.args = append([]string{fmt.Sprintf("bob/foo-%v", i)}, test.args...)
		stdout, stderr, code := run(c, c.MkDir(), "add-controller", test.args...)
		c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Equals, "")
		ctl, err := client.GetController(&params.GetController{
			EntityPath: params.EntityPath{
				User: "bob",
				Name: params.Name(fmt.Sprintf("foo-%v", i)),
			},
		})
		c.Assert(err, gc.IsNil)
		c.Assert(ctl.Location, gc.DeepEquals, test.location)
	}

}

var addControllerErrorTests = []struct {
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
	about:        "invalid controller name",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
	expectCode:   2,
}, {
	about:        "invalid name checked by controller",
	args:         []string{"bad!name/foo"},
	expectStderr: `invalid entity path "bad!name/foo": invalid user name "bad!name"`,
	expectCode:   2,
}, {
	about:        "invalid key",
	args:         []string{"bob/foo", "something"},
	expectStderr: `expected "key=value", got "something"`,
	expectCode:   2,
}}

func (s *addControllerSuite) TestAddControllerError(c *gc.C) {
	for i, test := range addControllerErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "add-controller", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
