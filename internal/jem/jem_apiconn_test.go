// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"time"

	cloudapi "github.com/juju/juju/api/cloud"
	corejujutesting "github.com/juju/juju/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemAPIConnSuite struct {
	corejujutesting.JujuConnSuite
	pool        *jem.Pool
	sessionPool *mgosession.Pool
	jem         *jem.JEM
}

var _ = gc.Suite(&jemAPIConnSuite{})

func (s *jemAPIConnSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(s.Session, 5)
	pool, err := jem.NewPool(jem.Params{
		DB:              s.Session.DB("jem"),
		ControllerAdmin: "controller-admin",
		SessionPool:     s.sessionPool,
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = s.pool.JEM()
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
}

func (s *jemAPIConnSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jemAPIConnSuite) TestPoolOpenAPI(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	info := s.APIInfo(c)
	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}

	err := s.jem.DB.AddController(ctl)
	c.Assert(err, gc.IsNil)

	// Open the API and check that it works.
	conn, err := s.jem.OpenAPI(ctlPath)
	c.Assert(err, gc.IsNil)
	s.assertConnectionAlive(c, conn)

	err = conn.Close()
	c.Assert(err, gc.IsNil)

	// Open it again and check that we get the
	// same cached connection.
	conn1, err := s.jem.OpenAPI(ctlPath)
	c.Assert(err, gc.IsNil)
	s.assertConnectionAlive(c, conn1)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)

	// Open it with OpenAPIFromDocs and check
	// that we still get the same connection.
	conn1, err = s.jem.OpenAPIFromDoc(ctl)
	c.Assert(err, gc.IsNil)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)

	// Close the JEM instance and check that the
	// connection is still alive, held open by the pool.
	s.jem.Close()
	s.assertConnectionAlive(c, conn)

	// Make sure the Close call is idempotent.
	s.jem.Close()
	s.assertConnectionAlive(c, conn)

	// Close the pool and make sure that the connection
	// has actually been closed this time.
	s.pool.Close()
	assertConnIsClosed(c, conn)

	// Check the close works again (we're just ensuring
	// that it doesn't panic here)
	s.pool.Close()
}

func (s *jemAPIConnSuite) TestPoolOpenAPIError(c *gc.C) {

	conn, err := s.jem.OpenAPI(params.EntityPath{"bob", "notthere"})
	c.Assert(err, gc.ErrorMatches, `cannot get controller: controller "bob/notthere" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(conn, gc.IsNil)
}

func assertConnIsClosed(c *gc.C, conn *apiconn.Conn) {
	select {
	case <-conn.Broken():
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for connection close")
	}
}

// assertConnectionAlive asserts that the given API
// connection is responding to requests.
func (s *jemAPIConnSuite) assertConnectionAlive(c *gc.C, conn *apiconn.Conn) {
	_, err := cloudapi.NewClient(conn).DefaultCloud()
	c.Assert(err, gc.IsNil)
}
