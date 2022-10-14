// Copyright 2020 Canonical Ltd.

package apiconn

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
)

// SupportsModelSummaryWatcher reports whether the controller supports
// the Controller.WatchAllModelSummaries method.
func (c *Conn) SupportsModelSummaryWatcher() bool {
	return c.HasFacadeVersion("Controller", 9)
}

// WatchAllModelSummaries initialises a new AllModelSummaryWatcher. On
// success the watcher ID is returned. If an error is returned it will be
// of type *APIError. This uses the WatchAllModelSummaries method on the
// Controller facade version 9.
func (c *Conn) WatchAllModelSummaries(_ context.Context) (string, error) {
	var resp jujuparams.SummaryWatcherID
	if err := c.APICall("Controller", 9, "", "WatchAllModelSummaries", nil, &resp); err != nil {
		return "", newAPIError(err)
	}
	return resp.WatcherID, nil
}

// ModelSummaryWatcherNext receives the next set of results from the
// model summary watcher with the given id. If an error is returned it
// will be of type *APIError. This uses the Next method on the
// ModelSummaryWatcher facade version 1.
func (c *Conn) ModelSummaryWatcherNext(_ context.Context, id string) ([]jujuparams.ModelAbstract, error) {
	var resp jujuparams.SummaryWatcherNextResults
	if err := c.APICall("ModelSummaryWatcher", 1, id, "Next", nil, &resp); err != nil {
		return nil, newAPIError(err)
	}
	return resp.Models, nil
}

// ModelSummaryWatcherStop stops the
// model summary watcher with the given id. If an error is returned it
// will be of type *APIError. This uses the Stop method on the
// ModelSummaryWatcher facade version 1.
func (c *Conn) ModelSummaryWatcherStop(_ context.Context, id string) error {
	if err := c.APICall("ModelSummaryWatcher", 1, id, "Stop", nil, nil); err != nil {
		return newAPIError(err)
	}
	return nil
}
