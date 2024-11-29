// Copyright 2024 Canonical.
package jujuclient2

import (
	"time"
	"unsafe"

	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type cacheDialerSuite struct {
	jimmtest.JujuSuite
}

var _ = gc.Suite(&cacheDialerSuite{})

func (s *cacheDialerSuite) getDialParams(c *gc.C) DialParams {
	info := s.APIInfo(c)
	return DialParams{
		ControllerTag: names.NewControllerTag(info.ControllerUUID),
		Addresses:     info.Addrs,
		CACertificate: info.CACert,
	}
}

func (s *cacheDialerSuite) TestCacheDialer(c *gc.C) {
	dialer := NewCacheDialer(JujuDialer{
		JWTService: s.JIMM.JWTService,
	}, time.Millisecond*500)

	p := s.getDialParams(c)

	// Dial controller
	conn, err := dialer.Dial(p)
	c.Assert(err, gc.IsNil)

	// Check conn is cached
	cachedConn, ok := dialer.conns[p.ControllerTag.Id()]
	c.Assert(ok, gc.Equals, true)
	c.Assert(conn, gc.DeepEquals, cachedConn)

	// Address check
	connAddr := (*[2]uintptr)(unsafe.Pointer(&conn))[1]
	cachedConnAddr := (*[2]uintptr)(unsafe.Pointer(&cachedConn))[1]
	c.Assert(connAddr, gc.Equals, cachedConnAddr)

	// Dial again and check cache is used
	conn2, err := dialer.Dial(p)
	c.Assert(err, gc.IsNil)

	// Address check
	conn2Addr := (*[2]uintptr)(unsafe.Pointer(&conn2))[1]
	c.Assert(connAddr, gc.Equals, conn2Addr)

	conn.Close()
	// Sleep double the cleanup to ensure it is cleaned up.
	time.Sleep(time.Second * 1)

	_, ok = dialer.conns[p.ControllerTag.Id()]
	c.Assert(ok, gc.Equals, false)

	// Dial a final time, and address should be different as it's a new conn
	// instance
	conn3, err := dialer.Dial(p)
	c.Assert(err, gc.IsNil)
	defer conn3.Close()

	conn3Addr := (*[2]uintptr)(unsafe.Pointer(&conn3))[1]
	c.Assert(connAddr, gc.Not(gc.Equals), conn3Addr)
}
