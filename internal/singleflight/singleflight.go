// Copyright 2017 Canonical Ltd.

package singleflight

import (
	"context"
	"sync"

	"gopkg.in/errgo.v1"
)

type flight struct {
	done   <-chan struct{}
	result interface{}
	err    error
}

// Group provides a location where a number of routines can wait for the
// outcome of a single in-flight call.
type Group struct {
	mu      sync.Mutex
	flights map[string]*flight
}

// Do starts the given function and waits until either the function
// completes or the context is done. If there is already an operation in
// flight with the same key then it will wait for that original function
// to complete. If the context is done before the given function
// completes then the function will not be stopped.
func (g *Group) Do(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.flights == nil {
		g.flights = make(map[string]*flight)
	}
	f := g.flights[key]
	if f == nil {
		ch := make(chan struct{})
		f = &flight{
			done: ch,
		}
		g.flights[key] = f
		go func() {
			f.result, f.err = fn()
			g.mu.Lock()
			defer g.mu.Unlock()
			delete(g.flights, key)
			close(ch)
		}()
	}
	g.mu.Unlock()
	select {
	case <-f.done:
		return f.result, errgo.Mask(f.err, errgo.Any)
	case <-ctx.Done():
		return nil, errgo.Mask(ctx.Err(), errgo.Any)
	}
}
