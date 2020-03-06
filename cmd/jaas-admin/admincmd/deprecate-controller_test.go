// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	"context"

	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type deprecateControllerSuite struct {
	commonSuite
}

var _ = gc.Suite(&deprecateControllerSuite{})

func (s *deprecateControllerSuite) TestRevoke(c *gc.C) {
	ctx := context.Background()

	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, replSetWarning)

	stdout, stderr, code = run(c, c.MkDir(), "deprecate-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that the deprecated status is set correctly
	// (rely on lower level testing to check that it's not chosen
	// when a new model is added).
	d, err := s.jemClient("bob").GetControllerDeprecated(ctx, &params.GetControllerDeprecated{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(d.Deprecated, gc.Equals, true)

	// Check that we can unset the deprecated status.
	stdout, stderr, code = run(c, c.MkDir(), "deprecate-controller", "--unset", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	d, err = s.jemClient("bob").GetControllerDeprecated(ctx, &params.GetControllerDeprecated{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(d.Deprecated, gc.Equals, false)
}

var deprecateControllerErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "no controller specified",
	args:         []string{},
	expectStderr: "no controller specified",
	expectCode:   2,
}, {
	about:        "too many arguments",
	args:         []string{"a", "b"},
	expectStderr: "too many arguments",
	expectCode:   2,
}, {
	about:        "only one part in path",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": need <user>/<name>`,
	expectCode:   2,
}}

func (s *deprecateControllerSuite) TestRevokeGetError(c *gc.C) {
	for i, test := range deprecateControllerErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "deprecate-controller", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
