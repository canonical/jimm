// Copyright 2021 Canonical Ltd.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// WatchAll initialises a new ModelWatcher. On success the watcher
// ID is returned. This uses the WatchAll method on the Client.
func (c Connection) WatchAll(ctx context.Context) (string, error) {
	const op = errors.Op("jujuclient.WatchAll")
	var resp jujuparams.AllWatcherId
	if err := c.client.Call(ctx, "Client", 1, "", "WatchAll", nil, &resp); err != nil {
		return "", errors.E(op, jujuerrors.Cause(err))
	}
	return resp.AllWatcherId, nil
}

// ModelWatcherNext receives the next set of results from the model
// watcher with the given id. This uses the Next method on the
// AllWatcher facade version 1.
func (c Connection) ModelWatcherNext(ctx context.Context, id string) ([]jujuparams.Delta, error) {
	const op = errors.Op("jujuclient.ModelWatcherNext")
	var resp jujuparams.AllWatcherNextResults
	if err := c.client.Call(ctx, "AllWatcher", 1, id, "Next", nil, &resp); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	return resp.Deltas, nil
}

// ModelWatcherStop stops the model watcher with the given id. This
// uses the Stop method on the AllWatcher facade version 1.
func (c Connection) ModelWatcherStop(ctx context.Context, id string) error {
	const op = errors.Op("jujuclient.ModelWatcherStop")
	if err := c.client.Call(ctx, "AllWatcher", 1, id, "Stop", nil, nil); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	return nil
}
