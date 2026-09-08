// Copyright 2025 Canonical.

package rpcproxy

import (
	"context"
	"encoding/json"
	"time"

	jujuTrace "github.com/juju/juju/core/trace"

	"github.com/canonical/jimm/v3/internal/telemetry"
)

// A message encodes a single message sent, or received, over an RPC
// connection. It contains the union of fields in a request or response
// message.
type message struct {
	// --- payload fields ---
	start     time.Time
	RequestID uint64          `json:"request-id,omitempty"`
	Type      string          `json:"type,omitempty"`
	Version   int             `json:"version,omitempty"`
	ID        string          `json:"id,omitempty"`
	Request   string          `json:"request,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Error     string          `json:"error,omitempty"`
	ErrorCode string          `json:"error-code,omitempty"`
	ErrorInfo map[string]any  `json:"error-info,omitempty"`
	Response  json.RawMessage `json:"response,omitempty"`
	// --- tracing fields ---
	span       *telemetry.TrackedSpan `json:"-"`
	TraceID    string                 `json:"trace-id,omitempty"`
	SpanID     string                 `json:"span-id,omitempty"`
	TraceFlags int                    `json:"trace-flags,omitempty"`
}

// startSpan continues the client's trace on this message and stores the
// resulting span.  It is a no-op when tracing is disabled or the message
// carries no incoming trace context.
func (m *message) startSpan(ctx context.Context) {
	if m == nil {
		return
	}

	// Continue the client's trace (no-op when TraceID is empty).
	traceCtx := jujuTrace.WithTraceScope(ctx, m.TraceID, m.SpanID, m.TraceFlags)

	_, childSpan := telemetry.StartSpan(traceCtx, "jimm.model-proxy",
		jujuTrace.StringAttr("rpc.facade", m.Type),
		jujuTrace.IntAttr("rpc.version", m.Version),
		jujuTrace.StringAttr("rpc.method", m.Request),
	)

	m.span = &childSpan

	// Write trace propagation headers so the controller can continue the trace.
	scope := childSpan.Scope()
	m.TraceID = scope.TraceID()
	m.SpanID = scope.SpanID()
	m.TraceFlags = scope.TraceFlags()
}

// finishSpan safely finishes the message's trace span (if any) and records
// the supplied error.  Safe to call on a nil receiver.
func (m *message) finishSpan(err error) {
	if m == nil {
		return
	}
	if m.span != nil {
		m.span.Finish(err)
		m.span = nil
	}
}
