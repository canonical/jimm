package apiconn

import (
	"sync"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/singleflight"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
)

// Cache holds a cache of connections to API servers.
//
// TODO(rog) implement cache size limits and cache deletion
// (for example when an API connection goes down).
type Cache struct {
	group  singleflight.Group
	params CacheParams
	mu     sync.Mutex
	// conns maps from model UUID to API connection.
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
			zapctx.Warn(context.TODO(), "cannot close API connection", zaputil.Error(err))
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

// OpenAPI dials the API server at the model with the given UUID.
// If a connection is not currently available, the dial
// function will be called to connect to it and its returned
// connection will be cached.. It is assumed that all
// connections to a given model UUID are equal. It is the
// responsibility of the caller to ensure this.
//
// The cause of any error returned from dial will be
// returned unmasked.
func (cache *Cache) OpenAPI(
	ctx context.Context,
	envUUID string,
	dial func() (api.Connection, *api.Info, error),
) (*Conn, error) {
	// First, a quick check to see whether the connection
	// is currrently cached.
	cache.mu.Lock()
	c := cache.conns[envUUID]
	if c != nil {
		select {
		case <-c.Broken():
			// The connection is broken. Evict it from the cache
			// and try dialling again.
			delete(cache.conns, envUUID)
			c.Close()
			c = nil
		default:
		}
	}
	cache.mu.Unlock()
	if c != nil {
		return c.Clone(), nil
	}
	// We use a singleflight.Group so that if several
	// clients ask for a connection to the same state
	// server with the same model UUID at the same time,
	// we only dial the controller once. The group is
	// keyed by the model UUID.
	x, err := cache.group.Do(ctx, envUUID, func() (interface{}, error) {
		st, stInfo, err := dial()
		if err != nil {
			return nil, errgo.Mask(err, errgo.Any)
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
		return nil, errgo.Mask(err, errgo.Any)
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

// APICall implements api.Connection.APICall by calling the underlying
// connection's APICall method and evicting the connection if the
// result indicates that there's a persistent upgrade-in-progress error.
// See https://bugs.launchpad.net/juju/+bug/1772381.
func (c *Conn) APICall(objType string, version int, id, request string, params, response interface{}) error {
	err := c.Connection.APICall(objType, version, id, request, params, response)
	if jujuparams.ErrCode(err) == jujuparams.CodeUpgradeInProgress {
		c.Evict()
	}
	return errgo.Mask(err, errgo.Any)
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

// Ping implements the erstwhile api.Connection.Ping() method.
func (c *Conn) Ping() error {
	return errgo.Mask(c.Connection.APICall("Pinger", 1, "", "Ping", nil, nil))
}
