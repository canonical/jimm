// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/params"
)

type revokeSuite struct {
	commonSuite
}

var _ = gc.Suite(&revokeSuite{})

func (s *revokeSuite) TestRevoke(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller. This also adds an model that we can
	// alter for our test.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can't get server or model.
	aliceClient := s.jemClient("alice")
	_, err := aliceClient.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "GET http://.*/v2/controller/bob/foo: unauthorized")
	_, err = aliceClient.GetModel(&params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "GET http://.*/v2/model/bob/foo: unauthorized")

	// Add alice to model permissions list.
	stdout, stderr, code = run(c, c.MkDir(),
		"grant",
		"bob/foo",
		"alice",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can get model but not server.
	_, err = aliceClient.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "GET http://.*/v2/controller/bob/foo: unauthorized")
	_, err = aliceClient.GetModel(&params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)

	// Add alice to server permissions list.
	stdout, stderr, code = run(c, c.MkDir(),
		"grant",
		"--controller",
		"bob/foo",
		"alice",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can now access the server.
	_, err = aliceClient.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)

	// Remove alice.
	stdout, stderr, code = run(c, c.MkDir(),
		"revoke",
		"bob/foo",
		"alice",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	bobClient := s.jemClient("bob")

	acl, err := bobClient.GetModelPerm(&params.GetModelPerm{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, params.ACL{
		Read: []string{},
	})
}

func (s *revokeSuite) TestRevokeChangeTemplatePerm(c *gc.C) {
	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new template.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--controller", "bob/foo", "bob/mytemplate", "controller=true", "apt-mirror=0.1.2.3")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that we can't read the template as another user
	// but we can as bob.
	aliceClient := s.jemClient("alice")
	_, err := aliceClient.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.ErrorMatches, "GET http://.*/v2/template/bob/mytemplate: unauthorized")

	bobClient := s.jemClient("bob")
	_, err = bobClient.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)

	// Change the permissions of the template to allow alice.
	stdout, stderr, code = run(c, c.MkDir(), "grant", "--template", "bob/mytemplate", "alice")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can now read the template.
	_, err = aliceClient.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)

	// Change the permissions of the template to revoke alice.
	stdout, stderr, code = run(c, c.MkDir(), "revoke", "--template", "bob/mytemplate", "alice")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice cannot now read the template.
	_, err = aliceClient.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.ErrorMatches, "GET http://.*/v2/template/bob/mytemplate: unauthorized")
}

var revokeErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "no model specified",
	args:         []string{},
	expectStderr: "no model or controller specified",
	expectCode:   2,
}, {
	about:        "no users specified",
	args:         []string{"bob/mymodel"},
	expectStderr: "no users specified",
	expectCode:   2,
}, {
	about:        "too many arguments",
	args:         []string{"a", "b", "c"},
	expectStderr: "too many arguments",
	expectCode:   2,
}, {
	about:        "only one part in path",
	args:         []string{"a", "b"},
	expectStderr: `invalid entity path "a": need <user>/<name>`,
	expectCode:   2,
}, {
	about:        "empty user name",
	args:         []string{"bob/b", "bob,"},
	expectStderr: `invalid value "bob,": empty user found`,
	expectCode:   2,
}, {
	about:        "invalid user name",
	args:         []string{"bob/b", "bob,!kung"},
	expectStderr: `invalid value "bob,!kung": invalid user name "!kung"`,
	expectCode:   2,
}, {
	about:        "--controller not allowed with --template",
	args:         []string{"--controller", "--template", "foo/bar", "bob"},
	expectStderr: `cannot specify both --controller and --template`,
	expectCode:   2,
}}

func (s *revokeSuite) TestRevokeGetError(c *gc.C) {
	for i, test := range revokeErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "revoke", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
