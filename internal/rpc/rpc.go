// Copyright 2021 Canonical Ltd.

// Package rpc implements the juju RPC protocol. The main difference
// between this implementation and the implementation in
// github.com/juju/juju/rpc is that this implementation allows canceling
// individual RPC calls using contexts. This implementation is a lot less
// flexible than the juju implementation.
package rpc

import "encoding/json"

// A message encodes a single message sent, or recieved, over an RPC
// connection. It contains the union of fields in a request or response
// message.
type message struct {
	RequestID uint64                 `json:"request-id,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Version   int                    `json:"version,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Request   string                 `json:"request,omitempty"`
	Params    json.RawMessage        `json:"params,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ErrorCode string                 `json:"error-code,omitempty"`
	ErrorInfo map[string]interface{} `json:"error-info,omitempty"`
	Response  json.RawMessage        `json:"response,omitempty"`
}

// isRequest returns whether the message is a request
func (m message) isRequest() bool {
	return m.Type != "" && m.Request != ""
}
