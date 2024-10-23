// Copyright 2024 Canonical.

package logger

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// HTTPLogFormatter is an implementation of chimiddleware.LogFormatter. It formats logs for http requests.
type HTTPLogFormatter struct{}

// httpLogEntry is an implementation of chimiddleware.LogEntry.
type httpLogEntry struct {
	*HTTPLogFormatter
	request *http.Request
}

// Write is called when the request handler has finished.
func (l *httpLogEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	fields := make([]zap.Field, 0)

	if status != 0 {
		fields = append(fields, zap.Int("status", status))
	}
	fields = append(fields,
		zap.String("method", l.request.Method),
		zap.String("path", l.request.RequestURI),
		zap.String("elapsed", elapsed.String()),
	)

	if status != 200 {
		zapctx.Debug(l.request.Context(), "request", fields...)
	} else {
		zapctx.Warn(l.request.Context(), "request", fields...)
	}

}

// Panic is called when the request handler panicked.
func (l *httpLogEntry) Panic(v interface{}, stack []byte) {
	middleware.PrintPrettyStack(v)
}

// NewLogEntry create the struct handling log entries.
func (l *HTTPLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return &httpLogEntry{
		HTTPLogFormatter: l,
		request:          r,
	}
}
