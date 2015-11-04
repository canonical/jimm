// Copyright 2015 Canonical Ltd.

package net

import (
	"io"
	"net"
	"time"

	"github.com/juju/utils/parallel"
	"gopkg.in/errgo.v1"
	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("jem-proxy.internal.net")

// Dialer is an interface that can be used to dial network endpoints.
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// Lookuper provides an interface for resolving names to one or more
// addresses.
type Lookuper interface {
	Lookup(name string) ([]string, error)
}

// ParallelDialer provides a Dial method that will attempt to dial a
// number of endpoints in parallel. The first successful connection is
// used.
type ParallelDialer struct {
	// MaxParallel is the maximum number of parallel Dial operations
	// to start at a time. If this is 0 then there is no limit.
	MaxParallel int

	// Interval is then length of time to wait in between launching
	// dial attempts. If Interval is 0 then the interval will be set
	// to 50ms.
	Interval time.Duration

	// Lookup is used to expand a name into a number of addresses to
	// Dial. If this is nil, then no lookup will be performed and a
	// single Dial operation will be created using the supplied
	// address.
	Lookuper Lookuper

	// Dialer is used to perform each Dial operation. If this is nil then
	// net.Dial will be used.
	Dialer Dialer
}

// Dial performs a parallel dial. The name is extracted from the supplied
// address and Lookup is performed on that name. If the addresses
// returned from Lookup do not have a port then the port from the
// original address will be added. Each address is then attempted in
// parallel until one succeeds.
func (d ParallelDialer) Dial(network, address string) (net.Conn, error) {
	var addrs []string
	if d.Lookuper != nil {
		var err error
		// Ignore the port address, lookup will find it.
		name, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errgo.Notef(err, "cannot resolve %q", address)
		}
		logger.Debugf("name: %q", name)
		addrs, err = d.Lookuper.Lookup(name)
		if err != nil {
			return nil, errgo.Notef(err, "cannot resolve %q", address)
		}
		for i, addr := range addrs {
			if _, _, err := net.SplitHostPort(addr); err != nil {
				addrs[i] = net.JoinHostPort(addr, port)
			}
		}
	} else {
		addrs = append(addrs, address)
	}
	if d.Interval == 0 {
		d.Interval = 50 * time.Millisecond
	}
	dialf := net.Dial
	if d.Dialer != nil {
		dialf = d.Dialer.Dial
	}
	try := parallel.NewTry(d.MaxParallel, nil)
	defer try.Kill()
	for _, addr := range addrs {
		addr := addr
		err := try.Start(func(<-chan struct{}) (io.Closer, error) {
			c, err := dialf(network, addr)
			if err != nil {
				logger.Warningf("cannot dial %q: %s", addr, err)
			}
			return c, err
		})
		if err == parallel.ErrStopped {
			break
		}
		select {
		case <-time.After(d.Interval):
		case <-try.Dead():
		}
	}
	try.Close()
	c, err := try.Result()
	if err != nil {
		return nil, errgo.Notef(err, "cannot connect to %q", address)
	}
	return c.(net.Conn), nil
}
