// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/params"
)

type changepermSuite struct {
	commonSuite
}

var _ = gc.Suite(&changepermSuite{})

func (s *changepermSuite) TestChangePerm(c *gc.C) {
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
		"change-perm",
		"--add-read",
		"alice",
		"bob/foo",
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
		"change-perm",
		"--controller",
		"--add-read",
		"alice",
		"bob/foo",
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

	// Add some more users and remove alice at the same time.
	stdout, stderr, code = run(c, c.MkDir(),
		"change-perm",
		"--add-read",
		"dave,charlie,edward",
		"--remove-read",
		"alice",
		"bob/foo",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	bobClient := s.jemClient("bob")

	// Set the users to a new set.
	stdout, stderr, code = run(c, c.MkDir(),
		"change-perm",
		"--set-read",
		"daisy,chloe,emily",
		"bob/foo",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	acl, err := bobClient.GetModelPerm(&params.GetModelPerm{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, params.ACL{
		Read: []string{"chloe", "daisy", "emily"},
	})
}

func (s *changepermSuite) TestChangeTemplatePerm(c *gc.C) {
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
	stdout, stderr, code = run(c, c.MkDir(), "change-perm", "--template", "--add-read", "alice", "bob/mytemplate")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can now read the template.
	_, err = aliceClient.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)
}

var changepermErrorTests = []struct {
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
	about:        "only one part in path",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
	expectCode:   2,
}, {
	about:        "empty user name",
	args:         []string{"--set-read", "bob,", "a/b"},
	expectStderr: `invalid value "bob," for flag --set-read: empty user found`,
	expectCode:   2,
}, {
	about:        "invalid user name",
	args:         []string{"--set-read", "bob,!kung", "a/b"},
	expectStderr: `invalid value "bob,!kung" for flag --set-read: invalid user name "!kung"`,
	expectCode:   2,
}, {
	about:        "--set-read not allowed with --add-read",
	args:         []string{"--set-read", "bob", "--add-read", "alice", "foo/bar"},
	expectStderr: `cannot specify --set-read with either --add-read or --remove-read`,
	expectCode:   2,
}, {
	about:        "--set-read not allowed with --remove-read",
	args:         []string{"--set-read", "bob", "--remove-read", "bob", "foo/bar"},
	expectStderr: `cannot specify --set-read with either --add-read or --remove-read`,
	expectCode:   2,
}, {
	about:        "--set-read not allowed with --remove-read even when empty",
	args:         []string{"--set-read", "", "--remove-read", "bob", "foo/bar"},
	expectStderr: `cannot specify --set-read with either --add-read or --remove-read`,
	expectCode:   2,
}, {
	about:        "--controller not allowed with --template",
	args:         []string{"--controller", "--template", "--set-read", "bob", "foo/bar"},
	expectStderr: `cannot specify both --controller and --template`,
	expectCode:   2,
}, {
	about:        "no permissions specified",
	args:         []string{"foo/bar"},
	expectStderr: `no permissions specified`,
	expectCode:   2,
}}

func (s *changepermSuite) TestGetError(c *gc.C) {
	for i, test := range changepermErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "change-perm", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
