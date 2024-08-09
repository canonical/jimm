// Copyright 2024 Canonical.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/errors"
)

// WatchAllModels initialises a new AllModelWatcher. On success the watcher
// ID is returned. This uses the WatchAllModels method on the Controller
// facade.
func (c Connection) WatchAllModels(ctx context.Context) (string, error) {
	const op = errors.Op("jujuclient.WatchAllModels")
	var resp jujuparams.SummaryWatcherID
	if err := c.CallHighestFacadeVersion(ctx, "Controller", []int{11, 7}, "", "WatchAllModels", nil, &resp); err != nil {
		return "", errors.E(op, jujuerrors.Cause(err))
	}
	return resp.WatcherID, nil
}

// AllModelWatcherNext receives the next set of results from the all-model
// watcher with the given id. This uses the Next method on the
// AllModelWatcher facade.
func (c Connection) AllModelWatcherNext(ctx context.Context, id string) ([]jujuparams.Delta, error) {
	const op = errors.Op("jujuclient.AllModelWatcherNext")
	var resp jujuparams.AllWatcherNextResults
	if err := c.CallHighestFacadeVersion(ctx, "AllModelWatcher", []int{4, 2}, id, "Next", nil, &resp); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	return resp.Deltas, nil
}

// AllModelWatcherStop stops the all-model watcher with the given id. This
// uses the Stop method on the AllModelWatcher facade.
func (c Connection) AllModelWatcherStop(ctx context.Context, id string) error {
	const op = errors.Op("jujuclient.AllModelWatcherStop")
	if err := c.CallHighestFacadeVersion(ctx, "AllModelWatcher", []int{4, 2}, id, "Stop", nil, nil); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	return nil
}
