// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/params"
)

type countSuite struct{}

var _ = gc.Suite(&countSuite{})

var countTests = []struct {
	about  string
	doc    params.Count
	n      int
	when   time.Time
	expect params.Count
}{{
	about: "from zero value",
	n:     5,
	when:  T(1000),
	expect: params.Count{
		Time:    T(1000),
		Current: 5,
		Max:     5,
		Total:   5,
	},
}, {
	about: "second time",
	doc: params.Count{
		Time:    T(1000),
		Current: 5,
		Max:     5,
		Total:   5,
	},
	n:    2,
	when: T(1500),
	expect: params.Count{
		Time:      T(1500),
		Current:   2,
		Max:       5,
		Total:     5,
		TotalTime: (1500 - 1000) * 5,
	},
}, {
	about: "count grows",
	doc: params.Count{
		Time:    T(1000),
		Current: 5,
		Max:     5,
		Total:   5,
	},
	n:    7,
	when: T(1500),
	expect: params.Count{
		Time:      T(1500),
		Current:   7,
		Max:       7,
		Total:     7,
		TotalTime: (1500 - 1000) * 5,
	},
}, {
	about: "total continues to grow",
	doc: params.Count{
		Time:    T(1000),
		Current: 5,
		Max:     10,
		Total:   50,
	},
	n:    7,
	when: T(1500),
	expect: params.Count{
		Time:      T(1500),
		Current:   7,
		Max:       10,
		Total:     52,
		TotalTime: (1500 - 1000) * 5,
	},
}, {
	about: "total time stays constant within a millisecond",
	doc: params.Count{
		Time:      T(1500),
		Current:   5,
		Max:       10,
		Total:     50,
		TotalTime: int64(time.Hour / time.Millisecond),
	},
	n:    5,
	when: T(1500).Add(time.Microsecond),
	expect: params.Count{
		Time:      T(1500),
		Current:   5,
		Max:       10,
		Total:     50,
		TotalTime: int64(time.Hour / time.Millisecond),
	},
}, {
	about: "total time continues to grow",
	doc: params.Count{
		Time:      T(1000),
		Current:   10,
		Max:       10,
		Total:     50,
		TotalTime: int64(time.Hour / time.Millisecond),
	},
	n:    20,
	when: T(5000),
	expect: params.Count{
		Time:      T(5000),
		Current:   20,
		Max:       20,
		Total:     60,
		TotalTime: int64(time.Hour/time.Millisecond) + (5000-1000)*10,
	},
}}

func (*countSuite) TestCount(c *gc.C) {
	for i, test := range countTests {
		c.Logf("test %d: %v", i, test.about)
		count := test.doc
		jem.UpdateCount(&count, test.n, test.when)
		count.Time = count.Time.UTC()
		c.Assert(count, jc.DeepEquals, test.expect)
	}
}
