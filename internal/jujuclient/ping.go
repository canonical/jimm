// Copyright 2024 Canonical.

package jujuclient

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
)

// Ping sends a ping message across the connection and waits for a
// response.
func (c Connection) Ping(ctx context.Context) error {
	const op = errors.Op("jujuclient.Ping")

	err := c.Call(ctx, "Pinger", 1, "", "Ping", nil, nil)
	if err != nil {
		err = errors.E(op, err)
	}
	return err
}
