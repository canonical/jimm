// Copyright 2026 Canonical.

package testing

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/client/usermanager"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestAddUser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	_, _, err := client.AddUser("bob", "Bob", "bob's super secret password")
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func TestRemoveUser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.RemoveUser("bob")
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func TestEnableUser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.EnableUser("bob")
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func TestDisableUser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.DisableUser("bob")
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func TestUserInfoAllUsers(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo(nil, usermanager.AllUsers)
	c.Assert(err, qt.Equals, nil)
	c.Assert(len(users), qt.Equals, 0)
}

func TestUserInfoSpecifiedUser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@canonical.com"}, usermanager.AllUsers)
	c.Assert(err, qt.Equals, nil)
	c.Assert(len(users), qt.Equals, 1)
	c.Assert(users[0].DateCreated.IsZero(), qt.Equals, false)
	users[0].DateCreated = time.Time{}
	users[0].LastConnection = nil
	c.Assert(users[0], qt.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@canonical.com",
		DisplayName: "alice",
		Access:      "",
	})
}

func TestUserInfoSpecifiedUsers(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@canonical.com", "bob@canonical.com"}, usermanager.AllUsers)
	c.Assert(err, qt.ErrorMatches, "bob@canonical.com: unauthorized")
	c.Assert(users, qt.HasLen, 0)
}

func TestUserInfoWithDomain(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice@mydomain", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@mydomain"}, usermanager.AllUsers)
	c.Assert(err, qt.Equals, nil)
	c.Assert(len(users), qt.Equals, 1)
	c.Assert(users[0].DateCreated.IsZero(), qt.Equals, false)
	users[0].DateCreated = time.Time{}
	c.Assert(users[0], qt.DeepEquals, jujuparams.UserInfo{
		Username:       "alice@mydomain",
		DisplayName:    "alice",
		Access:         "",
		LastConnection: users[0].LastConnection,
	})
}

func TestUserInfoInvalidUsername(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice-@canonical.com"}, usermanager.AllUsers)
	c.Assert(err, qt.ErrorMatches, `"alice-@canonical.com" is not a valid username`)
	c.Assert(users, qt.HasLen, 0)
}

func TestUserInfoLocalUsername(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice"}, usermanager.AllUsers)
	c.Assert(err, qt.ErrorMatches, `alice: unsupported local user; if this is a service account add @serviceaccount domain`)
	c.Assert(users, qt.HasLen, 0)
}

func TestSetPassword(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.SetPassword("bob", "bob's new super secret password")
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
