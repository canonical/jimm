// Copyright 2016 Canonical Ltd.

package jem

import (
	"io"

	"golang.org/x/net/context"
	errgo "gopkg.in/errgo.v1"
)

// errCancelled is the error returned when a context is cancelled before
// a Runner completes.
var errCanceled = errgo.New("canceled")

// runWithContext runs the given function and completes either when the
// runner completes, or when the given context is canceled. If Run
// returns because the context was cancelled then the returned error will
// have a cause of errCanceled.
func runWithContext(ctx context.Context, f func() (io.Closer, error)) (io.Closer, error) {
	type result struct {
		closer io.Closer
		err    error
	}
	ch := make(chan result)
	go func() {
		c, err := f()
		select {
		case ch <- result{c, err}:
		case <-ctx.Done():
			if err == nil {
				c.Close()
			} else {
				logger.Debugf("ignoring error in canceled task: %s", err)
			}
		}
	}()
	select {
	case r := <-ch:
		return r.closer, r.err
	case <-ctx.Done():
		return nil, errCanceled
	}
}
