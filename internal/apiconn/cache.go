package apiconn

import (
	"sync"

	"github.com/golang/groupcache/singleflight"
	"github.com/juju/juju/api"
	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
)

var logger = loggo.GetLogger("jem.internal.apiconn")

// Cache holds a cache of connections to API servers.
//
// TODO(rog) implement cache size limits and cache deletion
// (for example when an API connection goes down).
type Cache struct {
	group  singleflight.Group
	params CacheParams
	mu     sync.Mutex
	// conns maps from environment UUID to API connection.
	conns map[string]*Conn
}

// CacheParams holds parameters for a NewCache call.
type CacheParams struct {
	// TODO options for size limits and cache deletion.
}

// NewCache returns a new Cache that caches API server
// connections, using the given parameters to configure it.
func NewCache(p CacheParams) *Cache {
	return &Cache{
		params: p,
		conns:  make(map[string]*Conn),
	}
}

// Close implements io.Closer.Close, closing all open API connections.
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, st := range c.conns {
		// Don't bother returning the error because
		// an error on a closed API connection shouldn't
		// cause anything to fail. Just log the error instead.
		if err := st.Close(); err != nil {
			logger.Warningf("cannot close API connection: %v", err)
		}
	}
	return nil
}

// EvictAll clears the entire cache. This can be useful for testing.
func (cache *Cache) EvictAll() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for uuid, conn := range cache.conns {
		conn.Close()
		delete(cache.conns, uuid)
	}
}

// OpenAPI dials the API server at the environment with the given UUID.
// If a connection is not currently available, the dial
// function will be called to connect to it and its returned
// connection will be cached.. It is assumed that all
// connections to a given environment UUID are equal. It is the
// responsibility of the caller to ensure this.
func (cache *Cache) OpenAPI(
	envUUID string,
	dial func() (api.Connection, *api.Info, error),
) (*Conn, error) {
	// First, a quick check to see whether the connection
	// is currrently cached.
	cache.mu.Lock()
	c := cache.conns[envUUID]
	// TODO check that the connection is still healthy
	// and evict if not.
	cache.mu.Unlock()
	if c != nil {
		return c.Clone(), nil
	}
	// We use a singleflight.Group so that if several
	// clients ask for a connection to the same state
	// server with the same environment UUID at the same time,
	// we only dial the state server once. The group is
	// keyed by the environment UUID.
	x, err := cache.group.Do(envUUID, func() (interface{}, error) {
		st, stInfo, err := dial()
		if err != nil {
			return nil, errgo.Mask(err)
		}
		c := &Conn{
			// Note that we put the State and Info fields in the outer struct
			// so that it's clear in the godoc that they are available.
			// If godoc showed embedded exported fields,
			// it might be better to avoid doing that, and embed
			// sharedConn instead.
			Connection: st,
			Info:       stInfo,
			shared: &sharedConn{
				cache:    cache,
				uuid:     envUUID,
				refCount: 1,
			},
		}
		cache.mu.Lock()
		cache.conns[envUUID] = c
		cache.mu.Unlock()
		return c, nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return x.(*Conn).Clone(), nil
}

// Conn holds a connection to the API. It should be closed after use.
type Conn struct {
	// State holds the actual API connection. It should
	// not be closed directly.
	api.Connection

	// Info holds the information that was used to make
	// the connection.
	Info *api.Info

	closed bool
	shared *sharedConn
}

// sharedConn holds the shared connection value. Note that we maintain a
// separate data structure (Conn) so that we can ensure that Close is
// idempotent, defensively avoiding hard-to-find double-close errors.
type sharedConn struct {
	cache *Cache
	uuid  string

	mu       sync.Mutex
	refCount int
}

// Clone returns a new copy of the Conn which must be independently
// closed when finished with.
func (c *Conn) Clone() *Conn {
	c.shared.mu.Lock()
	defer c.shared.mu.Unlock()
	if c.closed {
		panic("clone of closed API connection")
	}
	c.shared.refCount++
	c1 := *c
	return &c1
}

// Close closes the API connection. This method is idempotent.
func (c *Conn) Close() error {
	c.shared.mu.Lock()
	defer c.shared.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.shared.refCount--; c.shared.refCount == 0 {
		return c.Connection.Close()
	}
	if c.shared.refCount < 0 {
		panic("negative ref count")
	}
	return nil
}

// Evict closes the connection and removes it from the cache.
func (c *Conn) Evict() {
	// We ignore close errors here as a very likely reason
	// we're calling Evict is because the connection is bad,
	// so errors are highly likely and not very useful.
	c.Close()
	cache := c.shared.cache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	// We only remove the entry from the cache if it still refers
	// to the same connection.
	uuid := c.shared.uuid
	if conn1 := cache.conns[uuid]; conn1 != nil && conn1.shared == c.shared {
		delete(cache.conns, uuid)
		conn1.Close()
	}
}
