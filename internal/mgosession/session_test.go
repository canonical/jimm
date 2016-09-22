// Copyright 2016 Canonical Ltd.

package mgosession_test

import (
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	mgo "gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/mgosession"
)

type suite struct {
	jujutesting.IsolatedMgoSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TestSession(c *gc.C) {
	pool := mgosession.NewPool(s.Session, 2)
	defer pool.Close()

	s0 := pool.Session()
	defer s0.Close()
	// The second session should be different because
	// two sessions are allowed.
	s1 := pool.Session()
	defer s1.Close()
	c.Assert(s1.Session, gc.Not(gc.Equals), s0.Session)

	// The third session should be reused.
	s2 := pool.Session()
	defer s2.Close()
	c.Assert(s2.Session, gc.Equals, s0.Session)
	c.Assert(s2, gc.Not(gc.Equals), s0)

	// Closing the first session should not affect the
	// third even though they're shared.
	s0.Close()
	c.Assert(s2.Ping(), gc.IsNil)

	// Closing is idempotent.
	s0.Close()
	c.Assert(s2.Ping(), gc.IsNil)

	// Closing the third session shouldn't actually
	// close the session because the pool still holds a reference
	// to it.
	s2.Close()
	c.Assert(s2.Ping(), gc.IsNil)

	// Closing the pool should finally close the
	// sessions that have no references (s0 and s2).
	pool.Close()

	assertClosed(c, s2.Session)

	// s1 has still not been closed, so still works.
	c.Assert(s1.Ping(), gc.IsNil)
	// After closing it, it will finally close down.
	s1.Close()
	assertClosed(c, s1.Session)
}

func (s *suite) TestDoNotReuse(c *gc.C) {
	pool := mgosession.NewPool(s.Session, 2)
	defer pool.Close()

	s0 := pool.Session()
	defer s0.Close()
	s0.DoNotReuse()

	s1 := pool.Session()
	defer s1.Close()

	// s0 was marked as DoNotReuse, so it should not
	// be handed out again.
	s2 := pool.Session()
	defer s2.Close()
	c.Assert(s2.Session, gc.Not(gc.Equals), s0.Session)

	// The next session wasn't marked as DoNotReuse,
	// so it will reuse s1.
	s3 := pool.Session()
	defer s3.Close()

	c.Assert(s3.Session, gc.Equals, s1.Session)
}

func (s *suite) TestClone(c *gc.C) {
	pool := mgosession.NewPool(s.Session, 2)
	defer pool.Close()

	s0 := pool.Session()
	s1 := s0.Clone()
	defer s1.Close()
	s0.Close()
	pool.Close()

	// Check that it still works - if we hadn't cloned s0, it would have been
	// closed when we closed it and the pool.
	c.Assert(s1.Ping(), gc.IsNil)

	// Check that when we finally close it, it is actually closed.
	s1.Close()
	assertClosed(c, s1.Session)
	assertClosed(c, s0.Session)
}

func assertClosed(c *gc.C, s *mgo.Session) {
	c.Assert(func() {
		s.Ping()
	}, gc.PanicMatches, `Session already closed`)
}
