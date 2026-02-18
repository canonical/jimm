// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/rpc/params"
)

// SupportsModelSummaryWatcher reports whether the controller supports
// the Controller.WatchAllModelSummaries method.
func (c Connection) SupportsModelSummaryWatcher() bool {
	return c.hasFacadeVersion("Controller", 11)
}

// SummaryWatcher defines the interface for watching model summaries.
type SummaryWatcher interface {
	Stop() error
	Next() ([]params.ModelAbstract, error)
}

// WatchAllModelSummaries initialises a new AllModelSummaryWatcher. On
// success the watcher ID is returned. If an error is returned it will be
// of type *APIError. This uses the WatchAllModelSummaries method on the
// Controller facade version 9.
func (c Connection) WatchAllModelSummaries(ctx context.Context) (SummaryWatcher, error) {
	return controller.NewClient(&c).WatchAllModelSummaries()
}
