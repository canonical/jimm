// Copyright 2015 Canonical Ltd.

package net_test

import (
	"fmt"
	"io"
	stdnet "net"

	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/cmd/jem-proxy/internal/net"
)

type dialSuite struct {
	server *EchoServer
}

var _ = gc.Suite(&dialSuite{})

func (s *dialSuite) SetUpSuite(c *gc.C) {
	var err error
	s.server, err = NewEchoServer()
	c.Assert(err, gc.IsNil)
}

func (s *dialSuite) TestParallelDialZeroValue(c *gc.C) {
	var d net.ParallelDialer
	conn, err := d.Dial("tcp", s.server.Address)
	c.Assert(err, gc.IsNil)
	n, err := conn.Write([]byte("TEST"))
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	resp := make([]byte, 1024)
	n, err = conn.Read(resp)
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	c.Assert(string(resp[:n]), gc.Equals, "TEST")
}

func (s *dialSuite) TestParallelDialUsesDialer(c *gc.C) {
	var d net.ParallelDialer
	d.Dialer = new(dialer)
	conn, err := d.Dial("tcp", s.server.Address)
	c.Assert(err, gc.IsNil)
	n, err := conn.Write([]byte("TEST"))
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	resp := make([]byte, 1024)
	n, err = conn.Read(resp)
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	c.Assert(string(resp[:n]), gc.Equals, "TEST")
	c.Assert(d.Dialer.(*dialer).network, gc.Equals, "tcp")
	c.Assert(d.Dialer.(*dialer).address, gc.Equals, s.server.Address)
}

type dialer struct {
	network string
	address string
}

func (d *dialer) Dial(network, address string) (stdnet.Conn, error) {
	d.network = network
	d.address = address
	return stdnet.Dial(network, address)
}

func (s *dialSuite) TestParallelDialUsesLookuper(c *gc.C) {
	var d net.ParallelDialer
	d.Lookuper = &lookuper{addresses: []string{s.server.Address}}
	conn, err := d.Dial("tcp", "not-there:noport")
	c.Assert(err, gc.IsNil)
	n, err := conn.Write([]byte("TEST"))
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	resp := make([]byte, 1024)
	n, err = conn.Read(resp)
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	c.Assert(string(resp[:n]), gc.Equals, "TEST")
	c.Assert(d.Lookuper.(*lookuper).name, gc.Equals, "not-there")
}

type lookuper struct {
	name      string
	addresses []string
	err       error
}

func (l *lookuper) Lookup(name string) ([]string, error) {
	l.name = name
	return l.addresses, l.err
}

type EchoServer struct {
	Address string
	l       stdnet.Listener
}

func (s *dialSuite) TestParallelDialAddsPort(c *gc.C) {
	var d net.ParallelDialer
	host, port, err := stdnet.SplitHostPort(s.server.Address)
	d.Lookuper = &lookuper{addresses: []string{host}}
	conn, err := d.Dial("tcp", "not-there:"+port)
	c.Assert(err, gc.IsNil)
	n, err := conn.Write([]byte("TEST"))
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	resp := make([]byte, 1024)
	n, err = conn.Read(resp)
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	c.Assert(string(resp[:n]), gc.Equals, "TEST")
	c.Assert(d.Lookuper.(*lookuper).name, gc.Equals, "not-there")
}

func (s *dialSuite) TestParallelMultipleAddresses(c *gc.C) {
	var d net.ParallelDialer
	d.Lookuper = &lookuper{addresses: []string{
		"localhost:0",
		s.server.Address,
		"localhost:0",
	}}
	conn, err := d.Dial("tcp", "not-there:noport")
	c.Assert(err, gc.IsNil)
	n, err := conn.Write([]byte("TEST"))
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	resp := make([]byte, 1024)
	n, err = conn.Read(resp)
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 4)
	c.Assert(string(resp[:n]), gc.Equals, "TEST")
	c.Assert(d.Lookuper.(*lookuper).name, gc.Equals, "not-there")
}

func (s *dialSuite) TestParallelDialError(c *gc.C) {
	var d net.ParallelDialer
	conn, err := d.Dial("tcp", s.server.Address+":bad-address")
	c.Assert(err, gc.ErrorMatches, `cannot connect to ".*:bad-address": dial tcp: too many colons in address .*:bad-address`)
	c.Assert(conn, gc.IsNil)
}

func (s *dialSuite) TestParallelAddressError(c *gc.C) {
	var d net.ParallelDialer
	d.Lookuper = &lookuper{addresses: []string{s.server.Address}}
	conn, err := d.Dial("tcp", "not-there:no-port:bad-address")
	c.Assert(err, gc.ErrorMatches, `cannot resolve "not-there:no-port:bad-address": too many colons in address not-there:no-port:bad-address`)
	c.Assert(conn, gc.IsNil)
}

func (s *dialSuite) TestParallelLookuplError(c *gc.C) {
	var d net.ParallelDialer
	d.Lookuper = &lookuper{err: fmt.Errorf("test error")}
	conn, err := d.Dial("tcp", "not-there:no-port")
	c.Assert(err, gc.ErrorMatches, `cannot resolve "not-there:no-port": test error`)
	c.Assert(conn, gc.IsNil)
}

func NewEchoServer() (*EchoServer, error) {
	l, err := stdnet.Listen("tcp", "")
	if err != nil {
		return nil, err
	}
	go func(l stdnet.Listener) {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go io.Copy(c, c)
		}
	}(l)
	return &EchoServer{
		Address: l.Addr().String(),
		l:       l,
	}, nil
}

func (s *EchoServer) Close() {
	s.l.Close()
}
