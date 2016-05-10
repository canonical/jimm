// Copyright 2016 Canonical Ltd.

package mongodoc

import (
	"time"
)

// TODO move this function into gopkg.in/mgo.v2/bson

// Time rounds the given time to millisecond precision
// in the same way that bson.Now rounds the current time.
func Time(t time.Time) time.Time {
	if t.IsZero() {
		// This special case is needed because the rounding algorithm
		// below fails if the time is outside UnixNano range.
		// We could use a more stable algorithm, e.g.
		//	t.Add(-(time.Duration(t.Nanosecond()) % time.Millisecond)).Local()
		// but it's important to be the same as bson so we use
		// exactly the same code as that package.
		return t
	}
	return time.Unix(0, t.UnixNano()/1e6*1e6)
}
