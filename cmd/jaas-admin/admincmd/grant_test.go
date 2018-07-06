// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/params"
)

type grantSuite struct {
	commonSuite
}

var _ = gc.Suite(&grantSuite{})

func (s *grantSuite) TestGrant(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller. This also adds an model that we can
	// alter for our test.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.addModel(c, "bob/foo", "bob/foo", "cred")

	stdout, stderr, code = run(c, c.MkDir(), "revoke", "--controller", "bob/foo", "everyone")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can't get controller or model.
	aliceClient := s.jemClient("alice")
	_, err := aliceClient.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "unauthorized")
	_, err = aliceClient.GetModel(&params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "unauthorized")

	// Add alice to model permissions list.
	stdout, stderr, code = run(c, c.MkDir(),
		"grant",
		"bob/foo",
		"alice",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can get model but not controller.
	_, err = aliceClient.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "unauthorized")
	_, err = aliceClient.GetModel(&params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)

	// Add alice to controller permissions list.
	stdout, stderr, code = run(c, c.MkDir(),
		"grant",
		"--controller",
		"bob/foo",
		"alice",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that alice can now access the controller.
	_, err = aliceClient.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)

	bobClient := s.jemClient("bob")

	// Set the users to a new set.
	stdout, stderr, code = run(c, c.MkDir(),
		"grant",
		"--set",
		"bob/foo",
		"daisy,chloe,emily@bar",
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
		Read: []string{"chloe", "daisy", "emily@bar"},
	})
}

var grantErrorTests = []struct {
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
	args:         []string{"bob/foo", "bob,"},
	expectStderr: `invalid value "bob,": empty user found`,
	expectCode:   2,
}, {
	about:        "invalid user name",
	args:         []string{"bob/foo", "bob,!kung"},
	expectStderr: `invalid value "bob,!kung": invalid user name "!kung"`,
	expectCode:   2,
}}

func (s *grantSuite) TestGetError(c *gc.C) {
	for i, test := range grantErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "grant", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}

func (s *grantSuite) TestGrantAdminACL(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.AddUser("alice")
	s.idmSrv.SetDefaultUser("bob")

	client := s.aclClient("bob")
	users, err := client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, gc.DeepEquals, []string{adminUser})

	// Update the admin ACL
	stdout, stderr, code := run(c, c.MkDir(), "grant", "--admin", "admin", "alice")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	users, err = client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, gc.DeepEquals, []string{adminUser, "alice"})
}

func (s *grantSuite) TestGrantAdminACLError(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	stdout, stderr, code := run(c, c.MkDir(), "grant", "--admin", "no-such-acl", "alice")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `(ERROR|error) .*: ACL not found`+"\n")
}

func (s *grantSuite) TestGrantAdminACLSet(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.AddUser("alice")
	s.idmSrv.SetDefaultUser("bob")

	client := s.aclClient("bob")
	users, err := client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, gc.DeepEquals, []string{adminUser})

	// Update the admin ACL
	stdout, stderr, code := run(c, c.MkDir(), "grant", "--admin", "--set", "admin", "alice,bob")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	users, err = client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, gc.DeepEquals, []string{"alice", "bob"})
}

func (s *grantSuite) TestGrantAdminACLSetError(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	stdout, stderr, code := run(c, c.MkDir(), "grant", "--admin", "--set", "no-such-acl", "alice")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `(ERROR|error) .*: ACL not found`+"\n")
}
