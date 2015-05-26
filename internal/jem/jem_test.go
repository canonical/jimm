// Copyright 2015 Canonical Ltd.

package jem_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemSuite struct {
	jujutesting.IsolatedMgoSuite
	pool  *jem.Pool
	store *jem.JEM
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool, err := jem.NewPool(
		s.Session.DB("jem"),
		&bakery.NewServiceParams{
			Location: "here",
		},
	)
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.store = s.pool.JEM()
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.store.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *jemSuite) TestAddStateServer(c *gc.C) {
	srv := &mongodoc.StateServer{
		Id:        "ignored",
		User:      "bob",
		Name:      "x",
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	env := &mongodoc.Environment{
		Id:            "ignored",
		User:          "ignored-user",
		Name:          "ignored-name",
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddStateServer(srv, env)
	c.Assert(err, gc.IsNil)

	// Check that the fields have been mutated as expected.
	c.Assert(srv, jc.DeepEquals, &mongodoc.StateServer{
		Id:        "bob/x",
		User:      "bob",
		Name:      "x",
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	})
	c.Assert(env, jc.DeepEquals, &mongodoc.Environment{
		Id:            "bob/x",
		User:          "bob",
		Name:          "x",
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		StateServer:   "bob/x",
	})

	srv1, err := s.store.StateServer("bob/x")
	c.Assert(err, gc.IsNil)
	c.Assert(srv1, jc.DeepEquals, &mongodoc.StateServer{
		Id:        "bob/x",
		User:      "bob",
		Name:      "x",
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	})
	env1, err := s.store.Environment("bob/x")
	c.Assert(err, gc.IsNil)
	c.Assert(env1, jc.DeepEquals, env)

	err = s.store.AddStateServer(srv, env)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *jemSuite) TestAddEnvironment(c *gc.C) {
	env := &mongodoc.Environment{
		Id:            "ignored",
		User:          "bob",
		Name:          "x",
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddEnvironment(env)
	c.Assert(err, gc.IsNil)
	c.Assert(env, jc.DeepEquals, &mongodoc.Environment{
		Id:            "bob/x",
		User:          "bob",
		Name:          "x",
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	})

	env1, err := s.store.Environment("bob/x")
	c.Assert(err, gc.IsNil)
	c.Assert(env1, jc.DeepEquals, env)

	err = s.store.AddEnvironment(env)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}
