// Copyright 2016 Canonical Ltd.

package mongodoc

import "time"

// EntityCount represents some kind of entity we
// want count over time.
// TODO when we expose counts in the API, this may well
// move into the params package.
type EntityCount string

const (
	UnitCount        EntityCount = "units"
	ApplicationCount EntityCount = "applications"
	MachineCount     EntityCount = "machines"
)

// Count records information about a changing count of
// of entities over time.
type Count struct {
	// Time holds the time when the count record was recorded.
	Time time.Time

	// Current holds the most recent count value,
	// recorded at the above time.
	Current int

	// MaxCount holds the maximum count recorded.
	Max int

	// Total holds the total number created over time.
	// This may be approximate if creation events are missed.
	Total int64

	// TotalTime holds the total time in milliseconds that any
	// entities have existed for. That is, if two entities have
	// existed for two seconds, this metric will record four
	// seconds.
	TotalTime int64
}

// Update updates the count document to record the count
// at the given time with respect to the current value
// of c.
func (c *Count) Update(count int, now time.Time) {
	// Round the time to mongo time (milliseconds)
	// so that we know we're working at the same granularity
	// as the storage of the time field.
	now = Time(now)
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
