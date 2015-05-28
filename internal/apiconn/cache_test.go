package apiconn_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/juju/api"
	corejujutesting "github.com/juju/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
)

type cacheSuite struct {
	corejujutesting.JujuConnSuite
	jem *jem.JEM
}

var _ = gc.Suite(&cacheSuite{})

func (s *cacheSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	pool, err := jem.NewPool(
		s.Session.DB("jem"),
		&bakery.NewServiceParams{
			Location: "here",
		},
	)
	c.Assert(err, gc.IsNil)
	s.jem = pool.JEM()
}

func (s *cacheSuite) TearDownTest(c *gc.C) {
	if s.jem != nil {
		s.jem.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *cacheSuite) TestOpenAPI(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	uuid := s.APIState.Client().EnvironmentUUID()
	conn, err := cache.OpenAPI(uuid, func() (*api.State, error) {
		return api.Open(s.APIInfo(c), api.DialOpts{})
	})
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Ping(), gc.IsNil)

	// If we close the connection, it should still remain around
	// in the cache.
	err = conn.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Ping(), gc.IsNil)

	// If we open the same uuid, we should get
	// the same connection without the dial
	// function being called.
	conn1, err := cache.OpenAPI(uuid, func() (*api.State, error) {
		c.Error("dial function called unexpectedly")
		return nil, fmt.Errorf("no")
	})
	c.Assert(conn1.State, gc.Equals, conn.State)
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

	dialFunc := func(uuid string, st *api.State) func() (*api.State, error) {
		return func() (*api.State, error) {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			callCounts[uuid]++
			return st, nil
		}
	}
	cache := apiconn.NewCache(apiconn.CacheParams{})
	fakes := []*api.State{{}, {}, {}}
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
			c.Check(conn.State, gc.Equals, st)
		}()
	}
	wg.Wait()
	c.Assert(callCounts, jc.DeepEquals, map[string]int{
		"uuid-0": 1,
		"uuid-1": 1,
		"uuid-2": 1,
	})
}

func (s *cacheSuite) TestOpenAPIError(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	conn, err := cache.OpenAPI("uuid", func() (*api.State, error) {
		return nil, fmt.Errorf("open error")
	})
	c.Assert(err, gc.ErrorMatches, "open error")
	c.Assert(conn, gc.IsNil)
}

func (s *cacheSuite) TestEvict(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	dialCount := 0
	dial := func() (*api.State, error) {
		dialCount++
		return api.Open(s.APIInfo(c), api.DialOpts{})
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

func assertConnIsClosed(c *gc.C, conn *apiconn.Conn) {
	select {
	case <-conn.State.RPCClient().Dead():
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for connection close")
	}
}
