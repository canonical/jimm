// Copyright 2020 Canonical Ltd.

// Package pubsub contains an implementation of a simple pubsub
// mechanism that passes messages about models between
// producers and subscribers.
package pubsub

import (
	"sync"

	"github.com/canonical/jimm/internal/errors"
	"github.com/juju/utils/v2/parallel"
)

// HandlerFunc takes two arguments - a model ID and the message about this model.
type HandlerFunc func(string, interface{})

type subscriber struct {
	matcher func(string) bool
	handler HandlerFunc
}

// Hub implements a simple pubsub mechanism that passes published
// messages about models to subscribers by calling their handler
// functions. It also stores last messages for all models - when
// a subscriber is added its handler function is called with last
// messages about all matching models.
type Hub struct {
	MaxConcurrency int

	mu          sync.Mutex
	parallel    *parallel.Run
	idx         int
	subscribers map[int]subscriber
	messages    map[string]interface{}
}

func (h *Hub) setupParallel() {
	if h.parallel == nil {
		maxConcurrency := h.MaxConcurrency
		if maxConcurrency == 0 {
			maxConcurrency = 100
		}
		h.parallel = parallel.NewRun(maxConcurrency)
	}
}

// Publish notifies all subscribers by calling their notify
// function.
func (h *Hub) Publish(model string, content interface{}) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	done := make(chan struct{})
	wait := sync.WaitGroup{}

	h.setupParallel()

	if h.messages == nil {
		h.messages = make(map[string]interface{})
	}
	h.messages[model] = content
	for _, s := range h.subscribers {
		if s.matcher(model) {
			wait.Add(1)
			h.parallel.Do(func() error {
				defer wait.Done()
				s.handler(model, content)
				return nil
			})
		}
	}

	go func() {
		wait.Wait()
		close(done)
	}()

	return done
}

// Subscribe to model information with a handler function. The handler
// function will be called whenever any message about this model is
// published.
func (h *Hub) Subscribe(model string, handler HandlerFunc) (func(), error) {
	return h.SubscribeMatch(modelMatches(model), handler)
}

// SubscribeMatch takes a function that determines whether the
// handler function should be called for a specific model.
// The matcher function should not do any "expensive" operations (ie
// doing a database lookup), because the pubsub hub will hold a lock
// while the modelMatcher function is called. Any DB lookups or other
// long operations for the matcher should be done out-of-band.
func (h *Hub) SubscribeMatch(modelMatcher func(string) bool, handler HandlerFunc) (func(), error) {
	const op = errors.Op("pubsub.SubscribeMatch")
	h.mu.Lock()
	defer h.mu.Unlock()

	if handler == nil {
		return func() {}, errors.E(op, "handler not specified")
	}
	if modelMatcher == nil {
		return func() {}, errors.E(op, "model matcher not specified")
	}
	idx := h.idx
	h.idx++
	s := subscriber{
		matcher: modelMatcher,
		handler: handler,
	}
	if h.subscribers == nil {
		h.subscribers = make(map[int]subscriber)
	}
	h.subscribers[idx] = s

	// check if there are any messages for matching models and
	// call the handler function if appropriate.
	for model, content := range h.messages {
		if modelMatcher(model) {
			handler(model, content)
		}
	}

	// return an unsubscribe function that removes
	// the subscriber from the list of active subscribers.
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subscribers, idx)
	}, nil
}

func modelMatches(matchModel string) func(string) bool {
	return func(model string) bool {
		return matchModel == model
	}
}
