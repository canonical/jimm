// Copyright 2024 Canonical.

package jimm

import (
	"sync/atomic"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestRunnerRun(t *testing.T) {
	c := qt.New(t)

	r := newRunner()

	var started, stopped int32
	doneC := make(chan struct{})
	f := func() {
		atomic.AddInt32(&started, 1)
		<-doneC
		atomic.AddInt32(&stopped, 1)
	}
	r.run("key1", f)
	r.run("key2", f)
	r.run("key1", f)
	close(doneC)
	r.wait()
	c.Check(atomic.LoadInt32(&started), qt.Equals, int32(2))
	c.Check(atomic.LoadInt32(&stopped), qt.Equals, int32(2))
}
