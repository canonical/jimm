// Copyright 2025 Canonical.

package jobtracker

import "time"

// ChangeRefreshInterval allows changing the refresh interval of the job tracker.
// This is useful for testing purposes to speed up the polling interval.
func ChangeRefreshInterval(tracker *Tracker, interval time.Duration) {
	// Change the refresh interval to a value less than the minimum.
	tracker.refreshInterval = interval
}
