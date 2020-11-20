// Copyright 2015 Canonical Ltd.

package admincmd_test

import (
	"context"

	gc "gopkg.in/check.v1"
)

type grantSuite struct {
	commonSuite
}

var _ = gc.Suite(&grantSuite{})

var grantErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "no acl specified",
	args:         []string{},
	expectStderr: "no administrative function specified",
	expectCode:   2,
}, {
	about:        "no users specified",
	args:         []string{"audit-log"},
	expectStderr: "no users specified",
	expectCode:   2,
}, {
	about:        "too many arguments",
	args:         []string{"a", "b", "c"},
	expectStderr: "too many arguments",
	expectCode:   2,
}, {
	about:        "empty user name",
	args:         []string{"audit-log", "bob,"},
	expectStderr: `invalid value "bob,": empty user found`,
	expectCode:   2,
}, {
	about:        "invalid user name",
	args:         []string{"audit-log", "bob,!kung"},
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
	stdout, stderr, code := run(c, c.MkDir(), "grant", "admin", "alice")
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

	stdout, stderr, code := run(c, c.MkDir(), "grant", "no-such-acl", "alice")
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
	stdout, stderr, code := run(c, c.MkDir(), "grant", "--set", "admin", "alice,bob")
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

	stdout, stderr, code := run(c, c.MkDir(), "grant", "--set", "no-such-acl", "alice")
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `(ERROR|error) .*: ACL not found`+"\n")
}
