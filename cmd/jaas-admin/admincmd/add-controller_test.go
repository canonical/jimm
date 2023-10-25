// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/params"
)

type addControllerSuite struct {
	commonSuite
}

var _ = gc.Suite(&addControllerSuite{})

func (s *addControllerSuite) TestAddController(c *gc.C) {
	ctx := context.Background()
	s.idmSrv.AddUser("bob", "admin")
	s.idmSrv.SetDefaultUser("bob")
	client := s.jemClient("bob")

	ctlPath := params.EntityPath{User: "bob", Name: "foo"}

	_, err := client.GetController(ctx, &params.GetController{
		EntityPath: ctlPath,
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	ctl, err := client.GetController(ctx, &params.GetController{
		EntityPath: ctlPath,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Location, gc.DeepEquals, map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName})
	c.Assert(ctl.Public, gc.DeepEquals, true)
	perm, err := client.GetControllerPerm(ctx, &params.GetControllerPerm{
		EntityPath: ctlPath,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(perm, jc.DeepEquals, params.ACL{
		Read: []string{"everyone"},
	})
}

// TODO(mhilton) test adding controllers with a DNS address. The previously
// existing test only worked because the connection was cached in the
// preceding test. Changes to the available certificate handling will be
// required before this can be done.

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
