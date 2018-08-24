// Copyright 2016 Canonical Ltd.

package jem

import (
	"time"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

// UpdateCount updates the count document to record the count
// at the given time with respect to the current value
// of c.
func UpdateCount(c *params.Count, count int, now time.Time) {
	// Round the time to mongo time (milliseconds)
	// so that we know we're working at the same granularity
	// as the storage of the time field.
	now = mongodoc.Time(now)
	if c.Time.IsZero() {
		c.Max = count
		c.Total = int64(count)
	} else {
		// We assume that the number of units has remained constant
		// between the last time the count was recorded and now.
		c.TotalTime += int64(now.Sub(c.Time)/time.Millisecond) * int64(c.Current)
		if count > c.Max {
			c.Max = count
		}
		if count > c.Current {
			c.Total += int64(count - c.Current)
		}
	}
	c.Current = count
	c.Time = now
}
