// Copyright 2016 Canonical Ltd.

package monitoring

import (
	"time"
)

// Request represents monitoring a request.
type Request struct {
	startTime time.Time
	label     string
}

// Reset the request monitor.
func (r *Request) Reset(path string) {
	r.label = path
	r.startTime = time.Now()
}

// ObserveMetric observes this metric.
func (r *Request) ObserveMetric() {
	requestDuration.WithLabelValues(r.label).Observe(float64(time.Since(r.startTime)) / float64(time.Microsecond))
}
