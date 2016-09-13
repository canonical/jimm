// Copyright 2015 Canonical Ltd.

package limitpool_test

import (
	"sync"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/limitpool"
)

type poolSuite struct{}

var _ = gc.Suite(&poolSuite{})

type item struct {
	g      limitpool.Gauge
	value  string
	closed bool
}

func (i *item) Close() {
	i.g.Dec()
	i.closed = true
}

type gauge struct {
	name string
	c    *gc.C
	mu   sync.Mutex
	n    int
}

func (g *gauge) Inc() {
	g.c.Logf("%s ++", g.name)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
}

func (g *gauge) Dec() {
	g.c.Logf("%s --", g.name)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n--
}

func (s *poolSuite) TestGetNoLimit(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 0,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestGetNoLimit",
			}
		},
		Allocated: allocated,
	})
	v := p.GetNoLimit().(*item)
	c.Assert(v.value, gc.Equals, "TestGetNoLimit")
	c.Assert(allocated.n, gc.Equals, 1)
}

func (s *poolSuite) TestGetSpareCapacity(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestGetSpareCapacity",
			}
		},
		Allocated: allocated,
	})
	v, err := p.Get(0)
	c.Assert(err, gc.IsNil)
	c.Assert(v.(*item).value, gc.Equals, "TestGetSpareCapacity")
	c.Assert(allocated.n, gc.Equals, 1)
}

func (s *poolSuite) TestGetTimeout(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 0,
		New: func(g limitpool.Gauge) limitpool.Item {
			c.Error("unexpected call to new")
			return nil
		},
		Allocated: allocated,
	})
	_, err := p.Get(0)
	c.Assert(err, gc.Equals, limitpool.ErrLimitExceeded)
	c.Assert(allocated.n, gc.Equals, 0)
}

func (s *poolSuite) TestGetWaiting(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	pooled := &gauge{
		name: "pooled",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestGetWaiting",
			}
		},
		Allocated: allocated,
		Pooled:    pooled,
	})
	v := p.GetNoLimit()
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.Put(v)
	}()
	v1, err := p.Get(5 * time.Second)
	c.Assert(err, gc.IsNil)
	c.Assert(v1, gc.Equals, v)
	c.Assert(allocated.n, gc.Equals, 1)
	c.Assert(pooled.n, gc.Equals, 0)
}

func (s *poolSuite) TestGetStored(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	pooled := &gauge{
		name: "pooled",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestGetStored",
			}
		},
		Allocated: allocated,
		Pooled:    pooled,
	})
	v := p.GetNoLimit()
	p.Put(v)
	c.Assert(pooled.n, gc.Equals, 1)
	c.Assert(allocated.n, gc.Equals, 1)
	v1, err := p.Get(0)
	c.Assert(err, gc.IsNil)
	c.Assert(v1, gc.Equals, v)
	c.Assert(pooled.n, gc.Equals, 0)
	c.Assert(allocated.n, gc.Equals, 1)
}

func (s *poolSuite) TestGetClosed(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 0,
		New: func(g limitpool.Gauge) limitpool.Item {
			c.Error("unexpected call to new")
			return nil
		},
		Allocated: allocated,
	})
	p.Close()
	_, err := p.Get(0)
	c.Assert(err, gc.Equals, limitpool.ErrClosed)
	c.Assert(allocated.n, gc.Equals, 0)
}

func (s *poolSuite) TestGetClosedDuringTimeout(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestGetClosedDuringTimeout",
			}
		},
		Allocated: allocated,
	})
	p.GetNoLimit()
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.Close()
	}()
	v, err := p.Get(5 * time.Second)
	c.Assert(err, gc.Equals, limitpool.ErrClosed)
	c.Assert(v, gc.Equals, nil)
	c.Assert(allocated.n, gc.Equals, 1)
}

func (s *poolSuite) TestGetNoLimitClosed(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestGetNoLimitClosed",
			}
		},
		Allocated: allocated,
	})
	p.Close()
	v := p.GetNoLimit()
	c.Assert(v.(*item).value, gc.Equals, "TestGetNoLimitClosed")
	c.Assert(allocated.n, gc.Equals, 1)
}

func (s *poolSuite) TestIncrementGaugePreventsAllocation(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 2,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestIncrementGaugePreventsAllocation",
			}
		},
		Allocated: allocated,
	})
	v, err := p.Get(0)
	c.Assert(v.(*item).value, gc.Equals, "TestIncrementGaugePreventsAllocation")
	c.Assert(allocated.n, gc.Equals, 1)
	v.(*item).g.Inc()
	c.Assert(allocated.n, gc.Equals, 2)
	_, err = p.Get(0)
	c.Assert(err, gc.Equals, limitpool.ErrLimitExceeded)
	v.(*item).g.Dec()
	c.Assert(allocated.n, gc.Equals, 1)
	v, err = p.Get(0)
	c.Assert(err, gc.IsNil)
	c.Assert(v.(*item).value, gc.Equals, "TestIncrementGaugePreventsAllocation")
	c.Assert(allocated.n, gc.Equals, 2)
}

func (s *poolSuite) TestPutClosesWhenClosed(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	pooled := &gauge{
		name: "pooled",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestPutClosesWhenClosed",
			}
		},
		Allocated: allocated,
		Pooled:    pooled,
	})
	v := p.GetNoLimit()
	p.Close()
	p.Put(v)
	c.Assert(v.(*item).closed, gc.Equals, true)
	c.Assert(allocated.n, gc.Equals, 0)
	c.Assert(pooled.n, gc.Equals, 0)
}

func (s *poolSuite) TestPutClosesWhenOverflowing(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	pooled := &gauge{
		name: "pooled",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 0,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestPutClosesWhenOverflowing",
			}
		},
		Allocated: allocated,
		Pooled:    pooled,
	})
	v := p.GetNoLimit()
	p.Put(v)
	c.Assert(v.(*item).closed, gc.Equals, true)
	c.Assert(allocated.n, gc.Equals, 0)
	c.Assert(pooled.n, gc.Equals, 0)
}

func (s *poolSuite) TestClosesClosesItemsInThePool(c *gc.C) {
	allocated := &gauge{
		name: "allocated",
		c:    c,
	}
	pooled := &gauge{
		name: "pooled",
		c:    c,
	}
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestPutClosesWhenOverflowing",
			}
		},
		Allocated: allocated,
		Pooled:    pooled,
	})
	v := p.GetNoLimit()
	p.Put(v)
	c.Assert(v.(*item).closed, gc.Equals, false)
	c.Assert(allocated.n, gc.Equals, 1)
	c.Assert(pooled.n, gc.Equals, 1)
	p.Close()
	c.Assert(v.(*item).closed, gc.Equals, true)
	c.Assert(allocated.n, gc.Equals, 0)
	c.Assert(pooled.n, gc.Equals, 0)
}

func (s *poolSuite) TestAllocatedAndPooledOptional(c *gc.C) {
	p := limitpool.New(limitpool.Params{
		Limit: 1,
		New: func(g limitpool.Gauge) limitpool.Item {
			return &item{
				g:     g,
				value: "TestAllocatedAndPooledOptional",
			}
		},
	})
	v := p.GetNoLimit()
	p.Put(v)
	v, err := p.Get(0)
	c.Assert(err, gc.IsNil)
	v2 := p.GetNoLimit()
	p.Put(v2)
	p.Put(v)
	p.Close()
}
