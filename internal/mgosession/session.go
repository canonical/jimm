// Copyright 2016 Canonical Ltd.

// mgosession provides multiplexing for MongoDB sessions. It is designed
// so that many concurrent operations can be performed without
// using one MongoDB socket connection for each operation.
package mgosession

import (
	"github.com/juju/mgosession"
	"golang.org/x/net/context"
	mgo "gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/zapctx"
)

// Pool represents a pool of mgo sessions.
type Pool struct {
	pool *mgosession.Pool
}

// NewPool returns a session pool that maintains a maximum
// of maxSessions sessions available for reuse.
func NewPool(ctx context.Context, s *mgo.Session, maxSessions int) *Pool {
	return &Pool{
		pool: mgosession.NewPool(ctxLogger{ctx}, s, maxSessions),
	}
}

// Session returns a session attached to the context or a session from
// the pool if one is not attached. The session must be closed when finished with.
func (p *Pool) Session(ctx context.Context) *mgo.Session {
	if s, ok := ctx.Value(contextSessionKey{}).(*mgo.Session); ok {
		return s.Clone()
	}
	return p.pool.Session(ctxLogger{ctx})
}

// ContextWithSession returns the given context with a *mgo.Session
// attached. Any subsequent calls to Session will return a clone of the
// attached session. the returned close function must be called when the
// context is finished with.
func (p *Pool) ContextWithSession(ctx context.Context) (_ context.Context, close func()) {
	if _, ok := ctx.Value(contextSessionKey{}).(*mgo.Session); ok {
		return ctx, func() {}
	}
	s := p.pool.Session(ctxLogger{ctx})
	return context.WithValue(ctx, contextSessionKey{}, s), s.Close
}

// Close closes the pool. It may be called concurrently with other
// Pool methods, but once called, a call to Session will panic.
func (p *Pool) Close() {
	p.pool.Close()
}

// Reset resets the session pool so that no existing
// sessions will be reused. This should be called
// when an unexpected error has been encountered using
// a session.
func (p *Pool) Reset() {
	p.pool.Reset()
}

type ctxLogger struct {
	ctx context.Context
}

func (l ctxLogger) Debug(msg string) {
	zapctx.Debug(l.ctx, msg)
}

func (l ctxLogger) Info(msg string) {
	zapctx.Info(l.ctx, msg)
}

type contextSessionKey struct{}
