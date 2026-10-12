// Copyright 2025 Canonical.

package ssh

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
)

// TestRelayHalfCloseDoesNotTruncate verifies that when one side half-closes
// its write direction, relay keeps copying the other direction to
// completion instead of tearing down both connections. It is a regression
// test for the truncation bug where either direction ending closed both
// connections.
//
// It uses a real TCP pair because CloseWrite must signal EOF per direction,
// which an in-memory net.Pipe cannot do.
func TestRelayHalfCloseDoesNotTruncate(t *testing.T) {
	c := qt.New(t)

	const payloadSize = 1 << 20 // 1 MiB, large enough to make truncation obvious.

	// a and b are the two ends relay bridges. client and server are the
	// test's handles to the far ends of each.
	client, a := tcpPair(c)
	b, server := tcpPair(c)

	// Fail instead of hanging if relay never delivers EOF.
	deadline := time.Now().Add(10 * time.Second)
	c.Assert(client.SetDeadline(deadline), qt.IsNil)
	c.Assert(server.SetDeadline(deadline), qt.IsNil)

	go relay(t.Context(), a, b)

	// The server drains until the client half-closes (EOF), then streams the
	// payload back. With full-close, the client's CloseWrite would tear down
	// this direction and fewer than payloadSize bytes would arrive.
	go func() {
		_, _ = io.Copy(io.Discard, server)
		_, _ = server.Write(make([]byte, payloadSize))
		_ = server.Close()
	}()

	// Half-close the client's write side, then read the full response.
	c.Assert(client.CloseWrite(), qt.IsNil)
	received, err := io.Copy(io.Discard, client)
	c.Assert(err, qt.IsNil)
	c.Check(received, qt.Equals, int64(payloadSize))
}

// TestRelayClosesOnDisconnect verifies that relay tears down both
// connections when the client disconnects, even if the controller never
// closes its side.
func TestRelayClosesOnDisconnect(t *testing.T) {
	c := qt.New(t)

	client, a := tcpPair(c)
	b, controller := tcpPair(c)

	// gliderlabs cancels the connection context when the client disconnects.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		relay(ctx, a, b)
		close(done)
	}()

	c.Assert(client.Close(), qt.IsNil)
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		c.Fatal("relay did not return after the client disconnected")
	}

	// The controller should see its connection closed, not left open.
	c.Assert(controller.SetReadDeadline(time.Now().Add(10*time.Second)), qt.IsNil)
	_, err := io.Copy(io.Discard, controller)
	c.Check(err, qt.IsNil)
}

// tcpPair returns a connected pair of TCP connections. Both support
// CloseWrite for per-direction half-close, unlike net.Pipe.
func tcpPair(c *qt.C) (*net.TCPConn, *net.TCPConn) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, qt.IsNil)
	defer l.Close()

	type accepted struct {
		conn net.Conn
		err  error
	}
	ch := make(chan accepted, 1)
	go func() {
		conn, err := l.Accept()
		ch <- accepted{conn, err}
	}()

	dialed, err := net.Dial("tcp", l.Addr().String())
	c.Assert(err, qt.IsNil)
	a := <-ch
	c.Assert(a.err, qt.IsNil)

	c.Cleanup(func() { _ = dialed.Close() })
	c.Cleanup(func() { _ = a.conn.Close() })
	return dialed.(*net.TCPConn), a.conn.(*net.TCPConn)
}
