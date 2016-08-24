// Copyright 2016 Canonical Ltd.

package servermon

import (
	"time"
)

// Request represents an API request that is being monitored.
// A request can only be used for a single API request at
// any one time.
type Request struct {
	startTime time.Time
	label     string
}

// Start should be called when an API request starts.
// The path holds the URL path to the API request.
func (r *Request) Start(path string) {
	r.label = path
	r.startTime = time.Now()
}

// End should be called when the API request completes.
// The Request value may then be reused for another
// API request.
func (r *Request) End() {
	requestDuration.WithLabelValues(r.label).Observe(float64(time.Since(r.startTime)) / float64(time.Microsecond))
}
