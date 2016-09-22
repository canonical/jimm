// Copyright 2016 Canonical Ltd.

// mgosession provides multiplexing for MongoDB sessions. It is designed
// so that many concurrent operations can be performed without
// using one MongoDB socket connection for each operation.
package mgosession

import (
	"sync"
	"sync/atomic"

	"github.com/juju/loggo"

	mgo "gopkg.in/mgo.v2"
)

var logger = loggo.GetLogger("jem.internal.mgosession")

// Pool represents a pool of mgo sessions.
type Pool struct {
	// mu guards the fields below it.
	mu sync.Mutex

	// sessions holds all the sessions currently available for use.
	sessions []*session

	// sessionIndex holds the index of the next session that will
	// be returned from Pool.Session.
	sessionIndex int

	// session holds the base session from which all sessions
	// returned from Pool.Session will be copied.
	session *mgo.Session

	// closed holds whether the Pool has been closed.
	closed bool
}

// NewPool returns a session pool that maintains a maximum
// of maxSessions sessions available for reuse.
func NewPool(s *mgo.Session, maxSessions int) *Pool {
	return &Pool{
		sessions: make([]*session, maxSessions),
		session:  s.Copy(),
	}
}

// Session returns a new session from the pool. It may
// reuse an existing session that has not been marked
// with DoNotReuse.
//
// Session may be called concurrently.
func (p *Pool) Session() *Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		panic("Session called on closed Pool")
	}
	s := p.sessions[p.sessionIndex]
	if s == nil || s.status.isDead() {
		if s != nil {
			// The session is marked as dead so close it.
			s.decRef()
		} else {
			logger.Debugf("creating new session %d", p.sessionIndex)
		}
		// Create a new session, either to replace an old one
		// or to fill the empty slot. Note that the refCount
		// is 1 because the entry in the pool counts as one reference.
		s = &session{
			session:  p.session.Copy(),
			refCount: 1,
		}
		p.sessions[p.sessionIndex] = s
	}
	p.sessionIndex = (p.sessionIndex + 1) % len(p.sessions)
	s.incRef()
	return &Session{
		s:       s,
		Session: s.session,
	}
}

// Close closes the pool. It may be called concurrently with other
// Pool methods, but once called, a call to Session will panic.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	for i, session := range p.sessions {
		if session != nil {
			session.decRef()
			p.sessions[i] = nil
		}
	}
	p.session.Close()
}

// Session wraps an mgo session object held in the pool.
type Session struct {
	// Session holds the embedded Session object.
	// This should not be closed directly.
	*mgo.Session

	s      *session
	closed bool
}

// Close closes the session. The embedded session should
// not be used after calling this method.
func (s *Session) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.s.decRef()
}

// Clone clones the session so that it can be closed
// independently.(for example if some concurrent process
// wishes to use the session independently). Note that it does
// not clone the embedded Session object.
func (s *Session) Clone() *Session {
	if s.closed {
		panic("cannot clone closed session")
	}
	s.s.incRef()
	return &Session{
		s:       s.s,
		// Use s.s.session instead of s.Session because there's no
		// possibility that someone might have changed it.
		Session: s.s.session,
	}
}

// DoNotReuse tags the Session object so that its
// session will not be subsequently returned by the Pool.Session
// method. Call this when you encounter a database
// error that seems like it's probably fatal to the session.
func (s *Session) DoNotReuse() {
	s.s.status.setDead()
}

// MayReuse reports whether the session has not yet
// marked as DoNotReuse. Note that calling DoNotReuse
// on any clone of the session will also cause this to return false.
func (s *Session) MayReuse() bool {
	return !s.s.status.isDead()
}

// session is the internal session type. This exists
// separately from Session solely so that the Session
// object can have an idempotent Close method.
type session struct {
	// refCount holds the number of Session objects that
	// currently refer to the session.
	refCount int32

	// status holds the status of the session.
	status sessionStatus

	// session holds the actual Session object.
	// This should not be closed directly.
	session *mgo.Session
}

func (s *session) incRef() {
	atomic.AddInt32(&s.refCount, 1)
}

func (s *session) decRef() {
	if atomic.AddInt32(&s.refCount, -1) == 0 {
		s.session.Close()
	}
}

// sessionStatus records the current status of a mgo session.
type sessionStatus int32

// setDead marks the session as dead, so that it won't be
// reused for new JEM instances.
func (s *sessionStatus) setDead() {
	atomic.StoreInt32((*int32)(s), 1)
}

// isDead reports whether the session has been marked as dead.
func (s *sessionStatus) isDead() bool {
	return atomic.LoadInt32((*int32)(s)) != 0
}
