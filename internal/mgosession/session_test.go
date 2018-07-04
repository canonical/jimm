// Copyright 2016 Canonical Ltd.

package mgosession_test

import (
	jujutesting "github.com/juju/testing"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/jemtest"
	"github.com/CanonicalLtd/jem/internal/mgosession"
)

type suite struct {
	jemtest.IsolatedMgoSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TestSession(c *gc.C) {
	psession := jujutesting.NewProxiedSession(c)
	defer psession.Close()
	pool := mgosession.NewPool(context.TODO(), psession.Session, 2)
	defer pool.Close()

	// Obtain a session from the pool, then kill its connection
	// so we can be sure that the next session is using a different
	// connection
	s0 := pool.Session(context.TODO())
	defer s0.Close()
	c.Assert(s0.Ping(), gc.IsNil)
	psession.CloseConns()
	c.Assert(s0.Ping(), gc.NotNil)

	// The next session should still work.
	s1 := pool.Session(context.TODO())
	defer s1.Close()
	c.Assert(s1.Ping(), gc.IsNil)

	// The third session should cycle back to the first
	// and fail.
	s2 := pool.Session(context.TODO())
	defer s2.Close()
	c.Assert(s2.Ping(), gc.NotNil)

	// Kill the connections again so that we
	// can be sure that the next session has been
	// copied.
	psession.CloseConns()
	c.Assert(s1.Ping(), gc.NotNil)

	// Resetting the pool should cause new sessions
	// to work again.
	pool.Reset()
	s3 := pool.Session(context.TODO())
	defer s3.Close()
	c.Assert(s3.Ping(), gc.IsNil)
	s4 := pool.Session(context.TODO())
	defer s4.Close()
	c.Assert(s4.Ping(), gc.IsNil)
}

func (s *suite) TestContextWithSession(c *gc.C) {
	psession := jujutesting.NewProxiedSession(c)
	defer psession.Close()
	pool := mgosession.NewPool(context.TODO(), psession.Session, 2)
	defer pool.Close()

	// Obtain a session from the pool, then kill its connection
	// so we can be sure that the next session is using a different
	// connection
	ctx, close := pool.ContextWithSession(context.TODO())
	defer close()
	s0 := pool.Session(ctx)
	defer s0.Close()
	c.Assert(s0.Ping(), gc.IsNil)
	psession.CloseConns()
	c.Assert(s0.Ping(), gc.NotNil)

	// The next session should return the same session from the
	// context.
	s1 := pool.Session(ctx)
	defer s1.Close()
	c.Assert(s1.Ping(), gc.NotNil)
}

func (s *suite) TestClosingPoolDoesNotClosePreviousSessions(c *gc.C) {
	pool := mgosession.NewPool(context.TODO(), s.Session, 2)
	session := pool.Session(context.TODO())
	defer session.Close()
	pool.Close()
	c.Assert(session.Ping(), gc.Equals, nil)
}
