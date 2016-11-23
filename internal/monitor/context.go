// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	"golang.org/x/net/context"
	tomb "gopkg.in/tomb.v2"
)

// newTombContext returns a context whose Done channel
// is closed when the tomb is killed.
//
// It includes values from the parent context but otherwise
// ignores its cancelable properties.
//
// TODO It would be nice to do better, but we've got a clash
// cultures between tombs and contexts here. Perhaps
// we should consider using golang.org/x/sync/errgroup
// instead of tombs.
func newTombContext(parent context.Context, tomb *tomb.Tomb) context.Context {
	return tombContext{
		parent: parent,
		done:   tomb.Dying(),
	}
}

type tombContext struct {
	parent context.Context
	done   <-chan struct{}
}

func (ctxt tombContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (ctxt tombContext) Value(key interface{}) interface{} {
	return ctxt.parent.Value(key)
}

func (ctxt tombContext) Done() <-chan struct{} {
	return ctxt.done
}

func (ctxt tombContext) Err() error {
	select {
	case <-ctxt.done:
		return context.Canceled
	default:
		return nil
	}
}
