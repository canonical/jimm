package apiconn_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/juju/api"
	corejujutesting "github.com/juju/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apiconn"
)

type cacheSuite struct {
	corejujutesting.JujuConnSuite
}

var _ = gc.Suite(&cacheSuite{})

func (s *cacheSuite) TestOpenAPI(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	uuid := s.APIState.Client().ModelUUID()
	var info *api.Info
	conn, err := cache.OpenAPI(uuid, func() (api.Connection, *api.Info, error) {
		info = s.APIInfo(c)
		return apiOpen(info, api.DialOpts{})
	})
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Ping(), gc.IsNil)
	c.Assert(conn.Info, gc.Equals, info)

	// If we close the connection, it should still remain around
	// in the cache.
	err = conn.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Ping(), gc.IsNil)

	// If we open the same uuid, we should get
	// the same connection without the dial
	// function being called.
	conn1, err := cache.OpenAPI(uuid, func() (api.Connection, *api.Info, error) {
		c.Error("dial function called unexpectedly")
		return nil, nil, fmt.Errorf("no")
	})
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn1.Ping(), gc.IsNil)

	// Check that Close is idempotent.
	err = conn1.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn1.Ping(), gc.IsNil)

	// When we close the cache, the connection should be finally closed.
	err = cache.Close()
	c.Assert(err, gc.IsNil)

	assertConnIsClosed(c, conn)
}

func (s *cacheSuite) TestConcurrentOpenAPI(c *gc.C) {
	var mu sync.Mutex
	callCounts := make(map[string]int)

	var info api.Info
	dialFunc := func(uuid string, st api.Connection) func() (api.Connection, *api.Info, error) {
		return func() (api.Connection, *api.Info, error) {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			callCounts[uuid]++
			return st, &info, nil
		}
	}
	cache := apiconn.NewCache(apiconn.CacheParams{})
	fakes := []api.Connection{&fakeConn{}, &fakeConn{}, &fakeConn{}}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			id := i % len(fakes)
			uuid := fmt.Sprint("uuid-", id)
			st := fakes[id%len(fakes)]
			conn, err := cache.OpenAPI(uuid, dialFunc(uuid, st))
			c.Check(err, gc.IsNil)
			c.Check(conn.Connection, gc.Equals, st)
		}()
	}
	wg.Wait()
	c.Assert(callCounts, jc.DeepEquals, map[string]int{
		"uuid-0": 1,
		"uuid-1": 1,
		"uuid-2": 1,
	})
}

type fakeConn struct {
	api.Connection
}

func (s *cacheSuite) TestOpenAPIError(c *gc.C) {
	apiErr := fmt.Errorf("open error")
	cache := apiconn.NewCache(apiconn.CacheParams{})
	conn, err := cache.OpenAPI("uuid", func() (api.Connection, *api.Info, error) {
		return nil, nil, apiErr
	})
	c.Assert(err, gc.ErrorMatches, "open error")
	c.Assert(errgo.Cause(err), gc.Equals, apiErr)
	c.Assert(conn, gc.IsNil)
}

func (s *cacheSuite) TestEvict(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	dialCount := 0
	dial := func() (api.Connection, *api.Info, error) {
		dialCount++
		return apiOpen(s.APIInfo(c), api.DialOpts{})
	}

	conn, err := cache.OpenAPI("uuid", dial)
	c.Assert(err, gc.IsNil)
	c.Assert(dialCount, gc.Equals, 1)

	// Try again just to sanity check that we're caching it.
	conn1, err := cache.OpenAPI("uuid", dial)
	c.Assert(err, gc.IsNil)
	c.Assert(dialCount, gc.Equals, 1)
	conn1.Close()

	// Evict the connection from the cache and check
	// that the connection has been closed and that
	// we make a new connection the next time.
	conn.Evict()

	assertConnIsClosed(c, conn)

	conn, err = cache.OpenAPI("uuid", dial)
	c.Assert(err, gc.IsNil)
	conn.Close()
	c.Assert(dialCount, gc.Equals, 2)
}

func (s *cacheSuite) TestEvictAll(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	conn, err := cache.OpenAPI("uuid0", func() (api.Connection, *api.Info, error) {
		return apiOpen(s.APIInfo(c), api.DialOpts{})
	})
	c.Assert(err, gc.IsNil)
	conn.Close()

	_, err = cache.OpenAPI("uuid1", func() (api.Connection, *api.Info, error) {
		return dummyConn{}, &api.Info{}, nil
	})
	cache.EvictAll()

	// Make sure that the connections are closed.
	assertConnIsClosed(c, conn)

	// Make sure both connections have actually been evicted.
	called := 0
	for i := 0; i < 2; i++ {
		_, err := cache.OpenAPI(fmt.Sprintf("uuid%d", i), func() (api.Connection, *api.Info, error) {
			called++
			return dummyConn{}, &api.Info{}, nil
		})
		c.Assert(err, gc.IsNil)
	}
	c.Assert(called, gc.Equals, 2)
}

type dummyConn struct {
	api.Connection
}

// apiOpen is like api.Open except that it also returns its
// info parameter.
func apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, *api.Info, error) {
	st, err := api.Open(info, opts)
	if err != nil {
		return nil, nil, err
	}
	return st, info, nil
}

func assertConnIsClosed(c *gc.C, conn api.Connection) {
	select {
	case <-conn.RPCClient().Dead():
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for connection close")
	}
}
