// Copyright 2024 Canonical.

package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/canonical/jimm/v3/internal/servermon"
)

// statusRecorder to record the status code from the ResponseWriter
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(statusCode int) {
	rec.statusCode = statusCode
	rec.ResponseWriter.WriteHeader(statusCode)
}

// MeasureHTTPResponseTime tracks response time of HTTP requests.
// We don't track websocket requests.
func MeasureHTTPResponseTime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the upgrade header because we only track http endpoints
		if r.Header.Get("Upgrade") == "websocket" {
			next.ServeHTTP(w, r)
			return
		}
		rec := statusRecorder{w, 200}
		start := time.Now()
		defer func() {
			route := chi.RouteContext(r.Context()).RoutePattern()
			statusCode := strconv.Itoa(rec.statusCode)
			servermon.ResponseTimeHistogram.WithLabelValues(route, r.Method, statusCode).Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(&rec, r)
	})
}
