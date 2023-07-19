// Copyright 2021 Canonical Ltd.

package jujuclient

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
)

// Ping sends a ping message accross the connection and waits for a
// response.
func (c Connection) Ping(ctx context.Context) error {
	const op = errors.Op("jujuclient.Ping")

	err := c.client.Call(ctx, "Pinger", 1, "", "Ping", nil, nil)
	if err != nil {
		err = errors.E(op, err)
	}
	return err
}
