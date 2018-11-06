package apiconn_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testserver"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
)

type cacheSuite struct {
	jemtest.JujuConnSuite
}

var _ = gc.Suite(&cacheSuite{})

func (s *cacheSuite) TestOpenAPI(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	uuid, ok := s.APIState.Client().ModelUUID()
	c.Assert(ok, gc.Equals, true)
	var info *api.Info
	conn, err := cache.OpenAPI(context.Background(), uuid, func() (api.Connection, *api.Info, error) {
		info = s.APIInfo(c)
		return apiOpen(info, api.DialOpts{})
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn.Ping(), gc.IsNil)
	c.Assert(conn.Info, gc.Equals, info)

	// If we close the connection, it should still remain around
	// in the cache.
	err = conn.Close()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn.Ping(), gc.IsNil)

	// If we open the same uuid, we should get
	// the same connection without the dial
	// function being called.
	conn1, err := cache.OpenAPI(context.Background(), uuid, func() (api.Connection, *api.Info, error) {
		c.Error("dial function called unexpectedly")
		return nil, nil, fmt.Errorf("no")
	})
	c.Assert(conn1.Connection, gc.Equals, conn.Connection)
	err = conn1.Close()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn1.Ping(), gc.IsNil)

	// Check that Close is idempotent.
	err = conn1.Close()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn1.Ping(), gc.IsNil)

	// When we close the cache, the connection should be finally closed.
	err = cache.Close()
	c.Assert(err, gc.Equals, nil)

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
			conn, err := cache.OpenAPI(context.Background(), uuid, dialFunc(uuid, st))
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

func (fakeConn) Broken() <-chan struct{} {
	return nil
}

func (s *cacheSuite) TestOpenAPIError(c *gc.C) {
	apiErr := fmt.Errorf("open error")
	cache := apiconn.NewCache(apiconn.CacheParams{})
	conn, err := cache.OpenAPI(context.Background(), "uuid", func() (api.Connection, *api.Info, error) {
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

	conn, err := cache.OpenAPI(context.Background(), "uuid", dial)
	c.Assert(err, gc.Equals, nil)
	c.Assert(dialCount, gc.Equals, 1)

	// Try again just to sanity check that we're caching it.
	conn1, err := cache.OpenAPI(context.Background(), "uuid", dial)
	c.Assert(err, gc.Equals, nil)
	c.Assert(dialCount, gc.Equals, 1)
	conn1.Close()

	// Evict the connection from the cache and check
	// that the connection has been closed and that
	// we make a new connection the next time.
	conn.Evict()

	assertConnIsClosed(c, conn)

	conn, err = cache.OpenAPI(context.Background(), "uuid", dial)
	c.Assert(err, gc.Equals, nil)
	conn.Close()
	c.Assert(dialCount, gc.Equals, 2)
}

func (s *cacheSuite) TestEvictAll(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	conn, err := cache.OpenAPI(context.Background(), "uuid0", func() (api.Connection, *api.Info, error) {
		return apiOpen(s.APIInfo(c), api.DialOpts{})
	})
	c.Assert(err, gc.Equals, nil)
	conn.Close()

	_, err = cache.OpenAPI(context.Background(), "uuid1", func() (api.Connection, *api.Info, error) {
		return fakeConn{}, &api.Info{}, nil
	})
	cache.EvictAll()

	// Make sure that the connections are closed.
	assertConnIsClosed(c, conn)

	// Make sure both connections have actually been evicted.
	called := 0
	for i := 0; i < 2; i++ {
		_, err := cache.OpenAPI(context.Background(), fmt.Sprintf("uuid%d", i), func() (api.Connection, *api.Info, error) {
			called++
			return fakeConn{}, &api.Info{}, nil
		})
		c.Assert(err, gc.Equals, nil)
	}
	c.Assert(called, gc.Equals, 2)
}

func (s *cacheSuite) TestOpenAPIWithBrokenConnection(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	c0 := &brokenConn{}
	conn, err := cache.OpenAPI(context.Background(), "uuid0", func() (api.Connection, *api.Info, error) {
		return c0, &api.Info{}, nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn.Connection, gc.Equals, c0)

	// Because the earlier connection is flagged as broken,
	// the next API open call will open another one.
	c1 := &fakeConn{}
	conn1, err := cache.OpenAPI(context.Background(), "uuid0", func() (api.Connection, *api.Info, error) {
		return c1, &api.Info{}, nil
	})
	c.Assert(conn1.Connection, gc.Equals, c1)
}

func (s *cacheSuite) TestContextCancel(c *gc.C) {
	cache := apiconn.NewCache(apiconn.CacheParams{})
	c0 := &fakeConn{}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan struct{})
	conn, err := cache.OpenAPI(ctx, "uuid0", func() (api.Connection, *api.Info, error) {
		cancel()
		<-ch
		return c0, &api.Info{}, nil
	})
	c.Assert(errgo.Cause(err), gc.Equals, context.Canceled)
	c.Assert(conn, gc.IsNil)
	close(ch)

	c1 := &fakeConn{}
	conn, err = cache.OpenAPI(context.Background(), "uuid0", func() (api.Connection, *api.Info, error) {
		cancel()
		<-ch
		return c1, &api.Info{}, nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(conn.Connection, gc.Equals, c0)
}

func (s *cacheSuite) TestEvictOnUpgradeInProgress(c *gc.C) {
	// Start a new API server so we control its upgrade-in-progress status.
	upgraded := make(chan struct{})

	config := testserver.DefaultServerConfig(c)
	config.UpgradeComplete = func() bool {
		select {
		case <-upgraded:
			return true
		default:
			return false
		}
	}
	srv := testserver.NewServerWithConfig(c, s.StatePool, config)
	defer srv.Stop()

	apiInfo := s.APIInfo(c)
	apiInfo.ModelTag = names.ModelTag{}
	apiInfo.Addrs = srv.Info.Addrs

	uuid := s.State.ControllerUUID()
	cache := apiconn.NewCache(apiconn.CacheParams{})
	dial := func() (api.Connection, *api.Info, error) {
		return apiOpen(apiInfo, api.DialOpts{})
	}

	callAPI := func(conn api.Connection) error {
		_, err := modelmanager.NewClient(conn).CreateModel(
			"name",
			apiInfo.Tag.Id(),
			"dummy",
			"",
			names.CloudCredentialTag{},
			nil,
		)
		return err
	}

	conn, err := cache.OpenAPI(context.Background(), uuid, dial)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = callAPI(conn)
	c.Check(params.ErrCode(err), gc.Equals, params.CodeUpgradeInProgress)

	// Try once again before upgrading, for luck.
	conn, err = cache.OpenAPI(context.Background(), uuid, dial)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = callAPI(conn)
	c.Check(params.ErrCode(err), gc.Equals, params.CodeUpgradeInProgress)

	// Close the upgraded channel, which will cause UpgradeComplete
	// to return true, which should mean that the next API connection
	// gets an unrestricted API, which should cause the API call to work.
	close(upgraded)

	conn, err = cache.OpenAPI(context.Background(), uuid, dial)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = callAPI(conn)
	c.Check(err, gc.Equals, nil)
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
	case <-conn.Broken():
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for connection close")
	}
}

type brokenConn struct {
	api.Connection
}

func (brokenConn) Broken() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
