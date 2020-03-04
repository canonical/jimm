// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type revokeSuite struct {
	commonSuite
}

var _ = gc.Suite(&revokeSuite{})

func (s *revokeSuite) TestRevoke(c *gc.C) {
	ctx := context.Background()

	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	// First add a controller. This also adds an model that we can
	// alter for our test.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, replSetWarning)
	s.addModel(ctx, c, "bob/foo", "bob/foo", "cred1")

	stdout, stderr, code = run(c, c.MkDir(), "revoke", "--controller", "bob/foo", "everyone")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.idmSrv.RemoveUsers()

	// Check that alice can't get controller or model.
	aliceClient := s.jemClient("alice")

	_, err := aliceClient.GetController(ctx, &params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, `Get http://.*/v2/controller/bob/foo: unauthorized`)

	_, err = aliceClient.GetModel(ctx, &params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, `Get http://.*/v2/model/bob/foo: unauthorized`)

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
	_, err = aliceClient.GetController(ctx, &params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.ErrorMatches, "Get .*/v2/controller/bob/foo: unauthorized")
	_, err = aliceClient.GetModel(ctx, &params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.Equals, nil)

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
	_, err = aliceClient.GetController(ctx, &params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.Equals, nil)

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

	acl, err := bobClient.GetModelPerm(ctx, &params.GetModelPerm{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, jc.DeepEquals, params.ACL{
		Read: []string{},
	})
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

func (s *revokeSuite) TestRevokeAdminACL(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.AddUser("alice")
	s.idmSrv.SetDefaultUser("bob")

	client := s.aclClient("bob")
	err := client.Set(context.Background(), "admin", []string{adminUser, "alice", "charlie"})
	c.Assert(err, gc.Equals, nil)

	// Update the admin ACL
	stdout, stderr, code := run(c, c.MkDir(), "revoke", "--admin", "admin", "alice")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	users, err := client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, gc.DeepEquals, []string{adminUser, "charlie"})
}

func (s *revokeSuite) TestRevokeAdminACLError(c *gc.C) {
	s.idmSrv.AddUser("bob", adminUser)
	s.idmSrv.SetDefaultUser("bob")

	stdout, stderr, code := run(c, c.MkDir(), "revoke", "--admin", "no-such-acl", "alice")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `(ERROR|error) .*: ACL not found`+"\n")
}
