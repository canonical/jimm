// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"time"

	"github.com/juju/juju/api/client/usermanager"
	jujuparams "github.com/juju/juju/rpc/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type userManagerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	_, _, err := client.AddUser("bob", "Bob", "bob's super secret password")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *userManagerSuite) TestRemoveUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.RemoveUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *userManagerSuite) TestEnableUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.EnableUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *userManagerSuite) TestDisableUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.DisableUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *userManagerSuite) TestUserInfoAllUsers(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo(nil, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 0)
}

func (s *userManagerSuite) TestUserInfoSpecifiedUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@canonical.com"}, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 1)
	c.Assert(users[0].DateCreated.IsZero(), gc.Equals, false)
	users[0].DateCreated = time.Time{}
	users[0].LastConnection = nil
	c.Assert(users[0], jc.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@canonical.com",
		DisplayName: "alice",
		Access:      "",
	})
}

func (s *userManagerSuite) TestUserInfoSpecifiedUsers(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@canonical.com", "bob@canonical.com"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, "bob@canonical.com: unauthorized access")
	c.Assert(users, gc.HasLen, 0)
}

func (s *userManagerSuite) TestUserInfoWithDomain(c *gc.C) {
	conn := s.open(c, nil, "alice@mydomain")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@mydomain"}, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 1)
	c.Assert(users[0].DateCreated.IsZero(), gc.Equals, false)
	users[0].DateCreated = time.Time{}
	c.Assert(users[0], jc.DeepEquals, jujuparams.UserInfo{
		Username:       "alice@mydomain",
		DisplayName:    "",
		Access:         "",
		LastConnection: users[0].LastConnection,
	})
}

func (s *userManagerSuite) TestUserInfoInvalidUsername(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice-@canonical.com"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, `"alice-@canonical.com" is not a valid username`)
	c.Assert(users, gc.HasLen, 0)
}

func (s *userManagerSuite) TestUserInfoLocalUsername(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, `alice: unsupported local user; if this is a service account add @serviceaccount domain`)
	c.Assert(users, gc.HasLen, 0)
}

func (s *userManagerSuite) TestSetPassword(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.SetPassword("bob", "bob's new super secret password")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
