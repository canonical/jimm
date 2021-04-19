// Copyright 2021 Canonical Ltd.

package jimm

import "sync"

// A runner ensures that only a single instance of a function, identified
// by a key, is running.
type runner struct {
	wg sync.WaitGroup

	mu      sync.Mutex
	running map[string]bool
}

// newRunner creates a new runner instance.
func newRunner() *runner {
	return &runner{
		running: make(map[string]bool),
	}
}

// run calls the given function in a new goroutine if there is no goroutine
// currently associated with the given key.
func (r *runner) run(key string, f func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running[key] {
		r.wg.Add(1)
		r.running[key] = true
		go func() {
			defer r.wg.Done()
			f()
			r.mu.Lock()
			defer r.mu.Unlock()
			delete(r.running, key)
		}()
	}

}

// wait blocks until all goroutines started by the runner have stopped.
func (r *runner) wait() {
	r.wg.Wait()
}
