// Copyright 2015 Canonical Ltd.

// Package limitpool provides functionality for limiting the
// total number of items in use at any one time (for example
// to limit the number of MongoDB sessions).
package limitpool

import (
	"errors"
	"sync"
	"time"
)

// ErrLimitExceeded is the error returned when the pool cannot retrieve
// an item because too many are currently in use.
var ErrLimitExceeded = errors.New("pool limit exceeded")

// ErrClosed is the error returned when the pool cannot retrieve
// an item because the pool has been closed.
var ErrClosed = errors.New("pool closed")

// Pool holds a pool of items and keeps track of the
// total number of allocated items.
type Pool struct {
	// params contains the configuration params for this pool.
	params Params

	// c is a buffered channel holding any
	// values in the pool that are not currently
	// in use.
	c chan Item

	// allocated is a gauge that records the number of allocated
	// items. If params.Allocated is non-nil then allocated wraps it.
	allocated gauge

	// mu guards the fields below it.
	mu sync.Mutex

	// closed holds whether the pool has been closed.
	closed bool
}

// Item represents an object that can be managed by the pool.
type Item interface {
	Close()
}

type Params struct {
	// Limit is the maximum number of allocated items at a time.
	Limit int64

	// New is called to create a new instance of the pool item when
	// required. The the provided Guage will have already been
	// incremented for the new Item, but should be decremented when
	// the item is closed and incremented if item makes any copies of
	// itself.
	New func(Gauge) Item

	// Allocated, if nono-nil, is a Gauge that will be incremented every time an
	// Item is allocated and decremented every time one is destroyed.
	Allocated Gauge

	// Pooled, if nono-nil,  is a Gauge that will be incremented every time an
	// Item is stored in the pool and decremented every time one is removed.
	Pooled Gauge
}

// New returns a new pool that imposes the given limit
// on the total number of allocated items.
//
// When a new item is required, new will be called to create it.
func New(params Params) *Pool {
	return &Pool{
		params: params,
		allocated: gauge{
			g: params.Allocated,
		},
		c: make(chan Item, params.Limit),
	}
}

// Close marks the pool as closed and closes all of the items in the
// pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	close(p.c)
	for v := range p.c {
		if p.params.Pooled != nil {
			p.params.Pooled.Dec()
		}
		v.Close()
	}
}

// Get retrieves an item from the pool. If the pool is currently empty
// and fewer than limit items are currently in circulation a new one will
// be created. If the limit has been reached then Get will wait for at
// least t before returng ErrLimitExceeded.
func (p *Pool) Get(t time.Duration) (Item, error) {
	v, err := p.get(false)
	if err == ErrLimitExceeded {
		select {
		case v, ok := <-p.c:
			if !ok {
				return nil, ErrClosed
			}
			if p.params.Pooled != nil {
				p.params.Pooled.Dec()
			}
			return v, nil
		case <-time.After(t):
		}
	}
	return v, err
}

func (p *Pool) get(noLimit bool) (Item, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		if !noLimit {
			return nil, ErrClosed
		}
		p.allocated.Inc()
		return p.params.New(&p.allocated), nil
	}
	select {
	case v := <-p.c:
		if p.params.Pooled != nil {
			p.params.Pooled.Dec()
		}
		return v, nil
	default:
	}
	if noLimit {
		p.allocated.Inc()
		return p.params.New(&p.allocated), nil
	} else if p.allocated.incLimit(p.params.Limit) {
		return p.params.New(&p.allocated), nil
	}
	return nil, ErrLimitExceeded
}

// GetNoLimit retrieve an item from the pool if one is available,
// otherwise it creates one immediately and returns it.
func (p *Pool) GetNoLimit() Item {
	v, err := p.get(true)
	if err != nil {
		// This should not be possible.
		panic(err)
	}
	return v
}

// Put puts v back into the pool. The item must previously have been
// returned from Get or GetNoLimit, or have been copied from an Item that
// was.
//
// If the number of allocated items exceeds the limit, x will be
// immediately closed.
func (p *Pool) Put(v Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.allocated.get() > p.params.Limit {
		v.Close()
		return
	}
	select {
	case p.c <- v:
		if p.params.Pooled != nil {
			p.params.Pooled.Inc()
		}
		return
	default:
		// This should be impossible (if n <= max then there must be room in the channel)
		// but it can be recovered by deleting.
	}
	v.Close()
}

type Gauge interface {
	// Inc increments the value of the gauge by 1.
	Inc()

	// Dec decrements the value of the gauge by 1.
	Dec()
}

type gauge struct {
	// g is a Gauge wrapped by this one, if it is nil no gauge is wrapped.
	g Gauge

	// mu protects the values below it.
	mu sync.Mutex

	// n is the value of the gauge
	n int64
}

// Inc implements Gauge.Inc.
func (g *gauge) Inc() {
	g.mu.Lock()
	g.n++
	g.mu.Unlock()
	if g.g != nil {
		g.g.Inc()
	}
}

// Dec implements Gauge.Dec.
func (g *gauge) Dec() {
	g.mu.Lock()
	g.n--
	g.mu.Unlock()
	if g.g != nil {
		g.g.Dec()
	}
}

// get gets the gauge value.
func (g *gauge) get() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.n
}

// incLimit increments the counter only if the value of n is lower than
// limit. If the counter is incremented then true is returned, otherwise
// false is returned.
func (g *gauge) incLimit(limit int64) bool {
	g.mu.Lock()
	if g.n >= limit {
		g.mu.Unlock()
		return false
	}
	g.n++
	g.mu.Unlock()
	if g.g != nil {
		g.g.Inc()
	}
	return true
}
