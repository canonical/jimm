// nolint:goheader
// Copyright 2009 The Go Authors.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//   - Redistributions of source code must retain the above copyright
//     notice, this list of conditions and the following disclaimer.
//   - Redistributions in binary form must reproduce the above
//     copyright notice, this list of conditions and the following disclaimer
// 	   in the documentation and/or other materials provided with the
//     distribution.
//   - Neither the name of Google LLC nor the names of its
//     contributors may be used to endorse or promote products derived from
//     this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package limitlistener

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimitListenerOverload(t *testing.T) {
	const (
		max      = 5
		attempts = max * 2
		msg      = "bye\n"
		timeout  = 10 * time.Millisecond
	)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l = ListenerWithTimeout(l, max, timeout)

	var wg sync.WaitGroup
	wg.Add(1)
	saturated := make(chan struct{})
	go func() {
		defer wg.Done()

		accepted := 0
		for {
			c, err := l.Accept()
			if err != nil {
				break
			}
			accepted++
			if accepted == max {
				close(saturated)
			}
			_, _ = io.WriteString(c, msg)

			// Leave c open until the listener is closed.
			defer c.Close()
		}
		t.Logf("with limit %d, accepted %d simultaneous connections", max, accepted)
		// The listener accounts open connections based on Listener-side Close
		// calls, so even if the client hangs up early (for example, because it
		// was a random dial from another process instead of from this test), we
		// should not end up accepting more connections than expected.
		if accepted != max {
			t.Errorf("want exactly %d", max)
		}
	}()

	dialCtx, cancelDial := context.WithCancel(context.Background())
	defer cancelDial()
	dialer := &net.Dialer{}

	var dialed, served int32
	var pendingDials sync.WaitGroup
	for n := attempts; n > 0; n-- {
		wg.Add(1)
		pendingDials.Add(1)
		go func() {
			defer wg.Done()

			c, err := dialer.DialContext(dialCtx, l.Addr().Network(), l.Addr().String())
			pendingDials.Done()
			if err != nil {
				t.Log(err)
				return
			}
			atomic.AddInt32(&dialed, 1)
			defer c.Close()

			// The kernel may queue more than max connections (allowing their dials to
			// succeed), but only max of them should actually be accepted by the
			// server. We can distinguish the two based on whether the listener writes
			// anything to the connection — a connection that was queued but not
			// accepted will be closed without transferring any data.
			if b, err := io.ReadAll(c); len(b) < len(msg) {
				t.Log(err)
				return
			}
			atomic.AddInt32(&served, 1)
		}()
	}

	// Give the server a bit of time after it saturates to make sure it doesn't
	// exceed its limit after serving this connection, then cancel the remaining
	// dials (if any).
	<-saturated
	time.Sleep(10 * time.Millisecond)
	cancelDial()
	// Wait for the dials to complete to ensure that the port isn't reused before
	// the dials are actually attempted.
	pendingDials.Wait()
	l.Close()
	wg.Wait()

	t.Logf("served %d simultaneous connections (of %d dialed, %d attempted)", served, dialed, attempts)

	// If some other process (such as a port scan or another test) happens to dial
	// the listener at the same time, the listener could end up burning its quota
	// on that, resulting in fewer than max test connections being served.
	// But the number served certainly cannot be greater.
	if served > max {
		t.Errorf("expected at most %d served", max)
	}
}

func TestLimitListenerSaturation(t *testing.T) {
	const (
		max             = 5
		attemptsPerWave = max * 2
		waves           = 10
		msg             = "bye\n"
		timeout         = 1 * time.Second
	)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l = ListenerWithTimeout(l, max, timeout)

	acceptDone := make(chan struct{})
	defer func() {
		l.Close()
		<-acceptDone
	}()
	go func() {
		defer close(acceptDone)

		var open, peakOpen int32
		var (
			saturated     = make(chan struct{})
			saturatedOnce sync.Once
		)
		var wg sync.WaitGroup
		for {
			c, err := l.Accept()
			if err != nil {
				break
			}
			if n := atomic.AddInt32(&open, 1); n > peakOpen {
				peakOpen = n
				if n == max {
					saturatedOnce.Do(func() {
						// Wait a bit to make sure the listener doesn't exceed its limit
						// after accepting this connection, then allow the in-flight
						// connections to write out and close.
						time.AfterFunc(10*time.Millisecond, func() { close(saturated) })
					})
				}
			}
			wg.Add(1)
			go func() {
				<-saturated
				_, _ = io.WriteString(c, msg)
				atomic.AddInt32(&open, -1)
				c.Close()
				wg.Done()
			}()
		}
		wg.Wait()

		t.Logf("with limit %d, accepted a peak of %d simultaneous connections", max, peakOpen)
		if peakOpen > max {
			t.Errorf("want at most %d", max)
		}
	}()

	for wave := 0; wave < waves; wave++ {
		var dialed, served int32
		var wg sync.WaitGroup
		for n := attemptsPerWave; n > 0; n-- {
			wg.Add(1)
			go func() {
				defer wg.Done()

				c, err := net.Dial(l.Addr().Network(), l.Addr().String())
				if err != nil {
					t.Log(err)
					return
				}
				atomic.AddInt32(&dialed, 1)
				defer c.Close()

				if b, err := io.ReadAll(c); len(b) < len(msg) {
					t.Log(err)
					return
				}
				atomic.AddInt32(&served, 1)
			}()
		}
		wg.Wait()

		t.Logf("served %d connections (of %d dialed, %d attempted)", served, dialed, attemptsPerWave)

		// Depending on the kernel's queueing behavior, we could get unlucky
		// and drop one or more connections. However, we should certainly
		// be able to serve at least max attempts out of each wave.
		// (In the typical case, the kernel will queue all of the connections
		// and they will all be served successfully.)
		if dialed < max {
			t.Errorf("expected at least %d dialed", max)
		}
		if served < dialed {
			t.Errorf("expected all dialed connections to be served")
		}
	}
}

// This test has been added to the existing set of limit listener
// tests and verifies that the listener continues to work after
// it has been saturated with the maximum number of connections.
// Once existing connection close, new connections should
// start to work.
func TestLimitListenerTimeout(t *testing.T) {
	const (
		max     = 5
		msg     = "bye\n"
		timeout = 10 * time.Millisecond
	)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l = ListenerWithTimeout(l, max, timeout)

	acceptDone := make(chan struct{})
	defer func() {
		l.Close()
		<-acceptDone
	}()
	go func() {
		defer close(acceptDone)

		var wg sync.WaitGroup
		// Loop max times
		// +1 for connection over the limit
		// +1 for connection after cleaning up previous connections
		for range max + 2 {
			c, err := l.Accept()
			if err != nil {
				t.Log(err)
				continue
			}
			wg.Add(1)
			go func() {
				_, _ = io.WriteString(c, msg)
				wg.Done()
				_, err := c.Read(make([]byte, 10))
				if err == nil || err.Error() != "EOF" {
					t.Logf("Expected read error but got: %v", err)
				}
				c.Close()
			}()
		}
		wg.Wait()
	}()

	connections := make([]net.Conn, 0, max)
	for range max {
		c, err := net.Dial(l.Addr().Network(), l.Addr().String())
		if err != nil {
			t.Error(err)
		}
		connections = append(connections, c)
		data := make([]byte, len(msg))
		if _, err := io.ReadAtLeast(c, data, len(msg)); string(data) != msg {
			t.Error(err)
		}
	}
	c, err := net.Dial(l.Addr().Network(), l.Addr().String())
	if err != nil {
		t.Error(err)
	}
	data := make([]byte, len(msg))
	expectedError := "EOF"
	if _, err := io.ReadAtLeast(c, data, len(msg)); err == nil || err.Error() != expectedError {
		t.Errorf("unexpected error received: got %v, want %s", err, expectedError)
	}
	err = c.Close()
	if err != nil {
		t.Error(err)
	}

	// Free up connections on the listener.
	for _, c := range connections {
		err := c.Close()
		if err != nil {
			t.Logf("failed to close connection: %s", err)
		}
	}

	c, err = net.Dial(l.Addr().Network(), l.Addr().String())
	if err != nil {
		t.Error(err)
	}
	if _, err := io.ReadAtLeast(c, data, len(msg)); string(data) != msg {
		t.Error(err)
	}
	c.Close()
}

type errorListener struct {
	net.Listener
}

func (errorListener) Accept() (net.Conn, error) {
	return nil, errFake
}

var errFake = errors.New("fake error from errorListener")

// This used to hang.
func TestLimitListenerError(t *testing.T) {
	const (
		n       = 2
		timeout = 10 * time.Millisecond
	)
	ll := ListenerWithTimeout(errorListener{}, n, timeout)
	for i := 0; i < n+1; i++ {
		_, err := ll.Accept()
		if err != errFake {
			t.Fatalf("Accept error = %v; want errFake", err)
		}
	}
}

func TestLimitListenerClose(t *testing.T) {
	const timeout = 10 * time.Millisecond
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ln = ListenerWithTimeout(ln, 1, timeout)

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		c, err := net.Dial(ln.Addr().Network(), ln.Addr().String())
		if err != nil {
			errCh <- err
			return
		}
		c.Close()
	}()

	c, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = <-errCh
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Allow the subsequent Accept to block before closing the listener.
	// (Accept should unblock and return.)
	timer := time.AfterFunc(10*time.Millisecond, func() { ln.Close() })

	c, err = ln.Accept()
	if err == nil {
		c.Close()
		t.Errorf("Unexpected successful Accept()")
	}
	if timer.Stop() {
		t.Errorf("Accept returned before listener closed: %v", err)
	}
}
