// Copyright 2024 Canonical.

package dbmodel

import "time"

// A RootKey is a macaroon root key.
type RootKey struct {
	ID        []byte
	CreatedAt time.Time
	Expires   time.Time
	RootKey   []byte
}
