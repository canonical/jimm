// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"
	"time"

	cloudapi "github.com/juju/juju/api/cloud"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type jemAPIConnSuite struct {
	jemtest.JujuConnSuite
	pool        *jem.Pool
	sessionPool *mgosession.Pool
	jem         *jem.JEM
}

var _ = gc.Suite(&jemAPIConnSuite{})

func (s *jemAPIConnSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:              s.Session.DB("jem"),
		ControllerAdmin: "controller-admin",
		SessionPool:     s.sessionPool,
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
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

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.IsNil)

	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}

	err = s.jem.DB.AddController(testContext, ctl, []mongodoc.CloudRegion{}, true)
	c.Assert(err, gc.IsNil)

	// Open the API and check that it works.
	conn, err := s.jem.OpenAPI(context.TODO(), ctlPath)
	c.Assert(err, gc.IsNil)
	s.assertConnectionAlive(c, conn)

	err = conn.Close()
	c.Assert(err, gc.IsNil)

	// Open it again and check that we get the
	// same cached connection.
	conn1, err := s.jem.OpenAPI(context.Background(), ctlPath)
	c.Assert(err, gc.IsNil)
	s.assertConnectionAlive(c, conn1)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)

	// Open it with OpenAPIFromDocs and check
	// that we still get the same connection.
	conn1, err = s.jem.OpenAPIFromDoc(context.Background(), ctl)
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
	assertConnClosed(c, conn)

	// Check the close works again (we're just ensuring
	// that it doesn't panic here)
	s.pool.Close()
}

func (s *jemAPIConnSuite) TestPoolOpenModelAPI(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.IsNil)

	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}
	err = s.jem.DB.AddController(testContext, ctl, []mongodoc.CloudRegion{}, true)
	c.Assert(err, gc.IsNil)

	mPath := params.EntityPath{"bob", "model"}
	m := &mongodoc.Model{
		Path:       mPath,
		UUID:       info.ModelTag.Id(),
		Controller: ctlPath,
	}
	err = s.jem.DB.AddModel(testContext, m)
	c.Assert(err, gc.IsNil)

	// Open the API and check that it works.
	conn, err := s.jem.OpenModelAPI(testContext, mPath)
	c.Assert(err, gc.IsNil)
	s.assertModelConnectionAlive(c, conn)

	err = conn.Close()
	c.Assert(err, gc.IsNil)

	// Open it again and check that we get the
	// same cached connection.
	conn1, err := s.jem.OpenModelAPI(testContext, mPath)
	c.Assert(err, gc.IsNil)
	s.assertModelConnectionAlive(c, conn1)
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)

	// Close the JEM instance and check that the
	// connection is still alive, held open by the pool.
	s.jem.Close()
	s.assertModelConnectionAlive(c, conn)

	// Make sure the Close call is idempotent.
	s.jem.Close()
	s.assertModelConnectionAlive(c, conn)

	// Close the pool and make sure that the connection
	// has actually been closed this time.
	s.pool.Close()
	assertConnClosed(c, conn)

	// Check the close works again (we're just ensuring
	// that it doesn't panic here)
	s.pool.Close()
}

func (s *jemAPIConnSuite) TestOpenAPIFromDocsCancel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.IsNil)

	ctl := &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}

	err = s.jem.DB.AddController(testContext, ctl, []mongodoc.CloudRegion{}, true)
	c.Assert(err, gc.IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := s.jem.OpenAPIFromDoc(ctx, ctl)
	c.Assert(errgo.Cause(err), gc.Equals, context.Canceled)
	c.Assert(conn, gc.IsNil)
}

func (s *jemAPIConnSuite) TestPoolOpenAPIError(c *gc.C) {
	conn, err := s.jem.OpenAPI(context.Background(), params.EntityPath{"bob", "notthere"})
	c.Assert(err, gc.ErrorMatches, `cannot get controller: controller "bob/notthere" not found`)
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
	_, err := cloudapi.NewClient(conn).DefaultCloud()
	c.Assert(err, gc.IsNil)
}

// assertModelConnectionAlive asserts that the given model API
// connection is responding to requests.
func (s *jemAPIConnSuite) assertModelConnectionAlive(c *gc.C, conn *apiconn.Conn) {
	_, err := conn.Client().ModelUserInfo()
	c.Assert(err, gc.IsNil)
}
