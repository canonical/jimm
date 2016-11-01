// Copyright 2016 Canonical Ltd.

package jujuapi

import "time"

type HeartMonitor heartMonitor

var (
	MakeCloud       = makeCloud
	MakeRegions     = makeRegions
	MergeRegions    = mergeRegions
	MergeStrings    = mergeStrings
	NewHeartMonitor = &newHeartMonitor
)

func InternalHeartMonitor(f func(time.Duration) HeartMonitor) func(time.Duration) heartMonitor {
	return func(d time.Duration) heartMonitor {
		return f(d)
	}
}
