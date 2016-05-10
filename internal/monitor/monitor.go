// Copyright 2016 Canonical Ltd.

// Package monitor provides monitoring for the controllers in JEM.
//
// We maintain a lease field
// in each controller which we hold as long as we monitor
// the controller so that we don't have multiple units redundantly
// monitoring the same controller.
package monitor

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
)

var logger = loggo.GetLogger("jem.internal.monitor")

const (
	// leaseExpiryDuration holds the length of time
	// a lease is acquired for.
	leaseExpiryDuration = time.Minute
)

// Clock holds the clock implementation used by the monitor.
// This is exported so it can be changed for testing purposes.
var Clock clock.Clock = clock.WallClock
