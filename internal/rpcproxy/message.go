// Copyright 2025 Canonical.

package rpcproxy

import (
	"encoding/json"
	"time"
)

// A message encodes a single message sent, or received, over an RPC
// connection. It contains the union of fields in a request or response
// message.
type message struct {
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
}
