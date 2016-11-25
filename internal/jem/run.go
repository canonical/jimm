// Copyright 2016 Canonical Ltd.

package jem

import (
	"io"

	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"golang.org/x/net/context"
)

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
				zapctx.Debug(ctx, "ignoring error in canceled task",
					zaputil.Error(err),
				)
			}
		}
	}()
	select {
	case r := <-ch:
		return r.closer, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
