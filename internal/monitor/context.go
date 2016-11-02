// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	"golang.org/x/net/context"
	tomb "gopkg.in/tomb.v2"
)

// newTombContext returns a context whose Done channel
// is closed when the tomb is killed.
func newTombContext(tomb *tomb.Tomb) context.Context {
	return tombContext{
		done: tomb.Dying(),
	}
}

type tombContext struct {
	done <-chan struct{}
}

func (ctxt tombContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (ctxt tombContext) Value(key interface{}) interface{} {
	return nil
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
