// Copyright 2021 Canoncial Ltd.

package dbmodel

import "time"

// A RootKey is a macaroon root key.
type RootKey struct {
	ID        []byte
	CreatedAt time.Time
	Expires   time.Time
	RootKey   []byte
}
