// Copyright 2024 Canonical.

package middleware

import (
	"net/http"
	"time"

	"github.com/canonical/jimm/v3/internal/servermon"
)

// MeasureResponseTime tracks response time of requests.
func MeasureResponseTime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		route := r.URL.Path
		servermon.ResponseTimeHistogram.WithLabelValues(route, r.Method).Observe(duration.Seconds())
	})
}
