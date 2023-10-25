// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"
	"time"

	cloudapi "github.com/juju/juju/api/cloud"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/apiconn"
	"github.com/canonical/jimm/internal/jem"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type jemAPIConnSuite struct {
	jemtest.JEMSuite
}

var _ = gc.Suite(&jemAPIConnSuite{})

func (s *jemAPIConnSuite) SetUpTest(c *gc.C) {
	s.JEMSuite.SetUpTest(c)
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
}

func (s *jemAPIConnSuite) TestPoolOpenAPI(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}

	err = s.JEM.DB.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	// Open the API and check that it works.
	conn, err := s.JEM.OpenAPI(context.TODO(), ctlPath)
	c.Assert(err, gc.Equals, nil)
	s.assertConnectionAlive(c, conn)

	err = conn.Close()
	c.Assert(err, gc.Equals, nil)

	// Open it again and check that we get the
	// same cached connection.
	conn1, err := s.JEM.OpenAPI(context.Background(), ctlPath)
	c.Assert(err, gc.Equals, nil)
	s.assertConnectionAlive(c, conn1)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.Equals, nil)

	// Open it with OpenAPIFromDocs and check
	// that we still get the same connection.
	conn1, err = s.JEM.OpenAPIFromDoc(context.Background(), ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.Equals, nil)

	// Close the JEM instance and check that the
	// connection is still alive, held open by the pool.
	s.JEM.Close()
	s.assertConnectionAlive(c, conn)

	// Make sure the Close call is idempotent.
	s.JEM.Close()
	s.assertConnectionAlive(c, conn)

	// Close the pool and make sure that the connection
	// has actually been closed this time.
	s.Pool.Close()
	assertConnClosed(c, conn)

	// Check the close works again (we're just ensuring
	// that it doesn't panic here)
	s.Pool.Close()
}

func (s *jemAPIConnSuite) TestPoolOpenModelAPI(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}
	err = s.JEM.DB.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	mPath := params.EntityPath{"bob", "model"}
	m := &mongodoc.Model{
		Path:       mPath,
		UUID:       info.ModelTag.Id(),
		Controller: ctlPath,
	}
	err = s.JEM.DB.InsertModel(testContext, m)
	c.Assert(err, gc.Equals, nil)

	// Open the API and check that it works.
	conn, err := s.JEM.OpenModelAPI(testContext, mPath)
	c.Assert(err, gc.Equals, nil)
	s.assertModelConnectionAlive(c, conn)

	err = conn.Close()
	c.Assert(err, gc.Equals, nil)

	// Open it again and check that we get the
	// same cached connection.
	conn1, err := s.JEM.OpenModelAPI(testContext, mPath)
	c.Assert(err, gc.Equals, nil)
	s.assertModelConnectionAlive(c, conn1)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.Equals, nil)

	// Close the JEM instance and check that the
	// connection is still alive, held open by the pool.
	s.JEM.Close()
	s.assertModelConnectionAlive(c, conn)

	// Make sure the Close call is idempotent.
	s.JEM.Close()
	s.assertModelConnectionAlive(c, conn)

	// Close the pool and make sure that the connection
	// has actually been closed this time.
	s.Pool.Close()
	assertConnClosed(c, conn)

	// Check the close works again (we're just ensuring
	// that it doesn't panic here)
	s.Pool.Close()
}

func (s *jemAPIConnSuite) TestOpenAPIFromDocsCancel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}

	err = s.JEM.DB.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := s.JEM.OpenAPIFromDoc(ctx, ctl)
	c.Assert(errgo.Cause(err), gc.Equals, context.Canceled)
	c.Assert(conn, gc.IsNil)
}

func (s *jemAPIConnSuite) TestPoolOpenAPIError(c *gc.C) {
	conn, err := s.JEM.OpenAPI(context.Background(), params.EntityPath{"bob", "notthere"})
	c.Assert(err, gc.ErrorMatches, `cannot get controller: controller not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(conn, gc.IsNil)
}

func assertConnClosed(c *gc.C, conn *apiconn.Conn) {
	select {
	case <-conn.Broken():
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for connection close")
	}
}

// assertConnectionAlive asserts that the given API
// connection is responding to requests.
func (s *jemAPIConnSuite) assertConnectionAlive(c *gc.C, conn *apiconn.Conn) {
	_, err := cloudapi.NewClient(conn).Clouds()
	c.Assert(err, gc.Equals, nil)
}

// assertModelConnectionAlive asserts that the given model API
// connection is responding to requests.
func (s *jemAPIConnSuite) assertModelConnectionAlive(c *gc.C, conn *apiconn.Conn) {
	_, err := conn.Client().ModelUserInfo()
	c.Assert(err, gc.Equals, nil)
}
