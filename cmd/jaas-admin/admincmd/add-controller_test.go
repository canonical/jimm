// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type addControllerSuite struct {
	commonSuite
}

var _ = gc.Suite(&addControllerSuite{})

var addControllerTests = []struct {
	about          string
	args           []string
	expectLocation map[string]string
	expectPublic   bool
}{{
	about:          "simple",
	args:           []string{},
	expectLocation: map[string]string{"cloud": "dummy", "region": "dummy-region"},
	expectPublic:   true,
}, {
	about:          "with api endpoint",
	args:           []string{"--public-address=localhost"},
	expectLocation: map[string]string{"cloud": "dummy", "region": "dummy-region"},
	expectPublic:   true,
}}

func (s *addControllerSuite) TestAddController(c *gc.C) {
	s.idmSrv.AddUser("bob", "admin")
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
		test.args = append(test.args, fmt.Sprintf("bob/foo-%v", i))
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
		c.Assert(ctl.Location, gc.DeepEquals, test.expectLocation)
		c.Assert(ctl.Public, gc.DeepEquals, test.expectPublic)
		if test.expectPublic {
			perm, err := client.GetControllerPerm(&params.GetControllerPerm{
				EntityPath: params.EntityPath{
					User: "bob",
					Name: params.Name(fmt.Sprintf("foo-%v", i)),
				},
			})
			c.Assert(err, gc.IsNil)
			c.Assert(perm, jc.DeepEquals, params.ACL{
				Read: []string{"everyone"},
			})
		}
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
	expectStderr: `invalid entity path "a": need <user>/<name>`,
	expectCode:   2,
}, {
	about:        "invalid name checked by controller",
	args:         []string{"bad!name/foo"},
	expectStderr: `invalid entity path "bad!name/foo": invalid user name "bad!name"`,
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
