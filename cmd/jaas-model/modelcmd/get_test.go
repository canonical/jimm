// Copyright 2015 Canonical Ltd.

package modelcmd_test

import (
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

type getSuite struct {
	commonSuite
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) TestGet(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller. This also adds an model that we can
	// get for our test.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(),
		"get",
		"bob/foo",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-foo:foo\n")

	// Check that we can attach to the new model
	// through the usual juju connection mechanism.
	store := jujuclient.NewFileClientStore()
	params, err := newAPIConnectionParams(
		store, "jem-foo", "", "foo", httpbakery.NewClient(),
	)
	c.Assert(err, gc.IsNil)
	client, err := juju.NewAPIConnection(params)
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
	about:        "only one part in model id",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": need <user>/<name>`,
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
