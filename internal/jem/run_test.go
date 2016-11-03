// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"io"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
)

type runSuite struct {
}

var _ = gc.Suite(&runSuite{})

func (s *runSuite) TestRunSuccess(c *gc.C) {
	ctx := context.Background()

	cl1 := testCloser{}
	cl, err := jem.RunWithContext(ctx, func() (io.Closer, error) {
		return &cl1, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cl, gc.Equals, &cl1)
	c.Assert(cl1.closed, gc.Equals, false)
}

func (s *runSuite) TestRunError(c *gc.C) {
	ctx := context.Background()
	err1 := errgo.New("test")
	cl, err := jem.RunWithContext(ctx, func() (io.Closer, error) {
		return nil, err1
	})
	c.Assert(err, gc.Equals, err1)
	c.Assert(cl, gc.IsNil)
}

func (s *runSuite) TestRunSuccessCanceled(c *gc.C) {
	var wg sync.WaitGroup
	ch := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	cl1 := make(chanCloser)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cl, err := jem.RunWithContext(ctx, func() (io.Closer, error) {
			ch <- struct{}{}
			<-ch
			return cl1, nil
		})
		c.Check(err, gc.Equals, jem.ErrCanceled)
		c.Check(cl, gc.Equals, nil)
	}()
	<-ch
	cancel()
	ch <- struct{}{}
	select {
	case <-cl1:
	case <-time.After(time.Second):
		c.Fatalf("timeout waiting for close")
	}
	wg.Wait()
}

func (s *runSuite) TestRunSuccessError(c *gc.C) {
	var wg sync.WaitGroup
	ch := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	err1 := errgo.New("test")
	wg.Add(1)
	go func() {
		defer wg.Done()
		cl, err := jem.RunWithContext(ctx, func() (io.Closer, error) {
			ch <- struct{}{}
			<-ch
			return nil, err1
		})
		c.Check(err, gc.Equals, jem.ErrCanceled)
		c.Check(cl, gc.Equals, nil)
	}()
	<-ch
	cancel()
	ch <- struct{}{}
	wg.Wait()
}

type testCloser struct {
	closed bool
}

func (c *testCloser) Close() error {
	c.closed = true
	return nil
}

type chanCloser chan struct{}

func (c chanCloser) Close() error {
	close(c)
	return nil
}
