// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"time"

	corejujutesting "github.com/juju/juju/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemAPIConnSuite struct {
	corejujutesting.JujuConnSuite
	pool  *jem.Pool
	store *jem.JEM
}

var _ = gc.Suite(&jemAPIConnSuite{})

func (s *jemAPIConnSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	pool, err := jem.NewPool(
		s.Session.DB("jem"),
		bakery.NewServiceParams{
			Location: "here",
		},
	)
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.store = s.pool.JEM()
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
}

func (s *jemAPIConnSuite) TearDownTest(c *gc.C) {
	s.store.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jemAPIConnSuite) TestPoolOpenAPI(c *gc.C) {
	info := s.APIInfo(c)
	srv := &mongodoc.StateServer{
		User:      "bob",
		Name:      "stateserver",
		HostPorts: info.Addrs,
		CACert:    info.CACert,
	}
	env := &mongodoc.Environment{
		UUID:          info.EnvironTag.Id(),
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}
	err := s.store.AddStateServer(srv, env)
	c.Assert(err, gc.IsNil)

	// Open the API and check that it works.
	conn, err := s.store.OpenAPI("bob/stateserver")
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Ping(), gc.IsNil)

	err = conn.Close()
	c.Assert(err, gc.IsNil)

	// Open it again and check that we get the
	// same cached connection.
	conn1, err := s.store.OpenAPI("bob/stateserver")
	c.Assert(err, gc.IsNil)
	c.Assert(conn1.Ping(), gc.IsNil)
	c.Assert(conn1.State, gc.Equals, conn.State)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)

	// Open it with OpenAPIFromDocs and check
	// that we still get the same connection.
	conn1, err = s.store.OpenAPIFromDocs(env, srv)
	c.Assert(err, gc.IsNil)
	c.Assert(conn1.State, gc.Equals, conn.State)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)

	// Close the JEM instance and check that the
	// connection is still alive, held open by the pool.
	s.store.Close()
	c.Assert(conn.Ping(), gc.IsNil)

	// Make sure the Close call is idempotent.
	s.store.Close()
	c.Assert(conn.Ping(), gc.IsNil)

	// Close the pool and make sure that the connection
	// has actually been closed this time.
	s.pool.Close()
	assertConnIsClosed(c, conn)

	// Check the close works again (we're just ensuring
	// that it doesn't panic here)
	s.pool.Close()
}

func (s *jemAPIConnSuite) TestPoolOpenAPIError(c *gc.C) {
	conn, err := s.store.OpenAPI("bob/notthere")
	c.Assert(err, gc.ErrorMatches, `cannot get environment: environment "bob/notthere" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(conn, gc.IsNil)

	// Insert an environment with a deliberately missing state server.
	env := &mongodoc.Environment{
		User:          "bob",
		Name:          "env",
		UUID:          "envuuid",
		AdminUser:     "admin",
		AdminPassword: "password",
		StateServer:   "noserver",
	}
	err = s.store.AddEnvironment(env)
	c.Assert(err, gc.IsNil)

	conn, err = s.store.OpenAPI("bob/env")
	c.Assert(err, gc.ErrorMatches, `cannot get state server for environment "envuuid": state server "noserver" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, err)
	c.Assert(conn, gc.IsNil)
}

func assertConnIsClosed(c *gc.C, conn *apiconn.Conn) {
	select {
	case <-conn.State.RPCClient().Dead():
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for connection close")
	}
}
