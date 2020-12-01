// Copyright 2020 Canonical Ltd.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// SupportsModelSummaryWatcher reports whether the controller supports
// the Controller.WatchAllModelSummaries method.
func (c Connection) SupportsModelSummaryWatcher() bool {
	return c.hasFacadeVersion("Controller", 9)
}

// WatchAllModelSummaries initialises a new AllModelSummaryWatcher. On
// success the watcher ID is returned. If an error is returned it will be
// of type *APIError. This uses the WatchAllModelSummaries method on the
// Controller facade version 9.
func (c Connection) WatchAllModelSummaries(_ context.Context) (string, error) {
	const op = errors.Op("jujuclient.WatchAllModelSummaries")
	var resp jujuparams.SummaryWatcherID
	if err := c.conn.APICall("Controller", 9, "", "WatchAllModelSummaries", nil, &resp); err != nil {
		return "", errors.E(op, jujuerrors.Cause(err))
	}
	return resp.WatcherID, nil
}

// ModelSummaryWatcherNext receives the next set of results from the
// model summary watcher with the given id. If an error is returned it
// will be of type *APIError. This uses the Next method on the
// ModelSummaryWatcher facade version 1.
func (c Connection) ModelSummaryWatcherNext(_ context.Context, id string) ([]jujuparams.ModelAbstract, error) {
	const op = errors.Op("jujuclient.ModelSummaryWatcherNext")
	var resp jujuparams.SummaryWatcherNextResults
	if err := c.conn.APICall("ModelSummaryWatcher", 1, id, "Next", nil, &resp); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	return resp.Models, nil
}

// ModelSummaryWatcherStop stops the
// model summary watcher with the given id. If an error is returned it
// will be of type *APIError. This uses the Stop method on the
// ModelSummaryWatcher facade version 1.
func (c Connection) ModelSummaryWatcherStop(_ context.Context, id string) error {
	const op = errors.Op("jujuclient.ModelSummaryWatcherStop")
	if err := c.conn.APICall("ModelSummaryWatcher", 1, id, "Stop", nil, nil); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	return nil
}
