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
		// Check the upgrade header because we only track http endpoints
		if r.Header.Get("Upgrade") != "websocket" {
			start := time.Now()
			defer func() {
				servermon.ResponseTimeHistogram.WithLabelValues(r.URL.Path, r.Method).Observe(time.Since(start).Seconds())
			}()
		}
		next.ServeHTTP(w, r)
	})
}
