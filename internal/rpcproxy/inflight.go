// Copyright 2025 Canonical.

package rpcproxy

import (
	"sync"
	"time"

	"github.com/canonical/jimm/v3/internal/servermon"
)

// inflightTracker holds only request messages that are still pending a response
// from a Juju controller, plus the replayable login message used for access-required retries.
type inflightTracker struct {
	controllerUUID string

	mu           sync.Mutex
	loginMessage *message
	requests     map[uint64]*message
}

func newInflightTracker() *inflightTracker {
	return &inflightTracker{requests: make(map[uint64]*message)}
}

func (tracker *inflightTracker) setControllerUUID(uuid string) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.controllerUUID = uuid
}

func (tracker *inflightTracker) controllerID() string {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	return tracker.controllerUUID
}

func (tracker *inflightTracker) rememberLogin(msg *message) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.loginMessage = msg
}

func (tracker *inflightTracker) login() *message {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	return tracker.loginMessage
}

func (tracker *inflightTracker) track(msg *message) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	msg.start = time.Now()
	tracker.requests[msg.RequestID] = msg
}

func (tracker *inflightTracker) request(requestID uint64) *message {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	msg, ok := tracker.requests[requestID]
	if !ok {
		return nil
	}
	return msg
}

// finish deletes the request message that corresponds to the response message ID.
func (tracker *inflightTracker) finish(requestID uint64) {
	tracker.mu.Lock()
	req, ok := tracker.requests[requestID]
	if ok {
		delete(tracker.requests, requestID)
	}
	controllerUUID := tracker.controllerUUID
	tracker.mu.Unlock()

	if ok {
		servermon.JujuCallDurationHistogram.WithLabelValues(
			req.Type,
			req.Request,
			controllerUUID,
		).Observe(time.Since(req.start).Seconds())
	}
}
