// Copyright 2024 Canonical.
package jujuclient2

import (
	"sync"
	"time"

	"github.com/juju/juju/api"
	"golang.org/x/sync/singleflight"
)

// cacheJujuDialer caches connections and if one is broken, attempts to re-establish it.
type cacheJujuDialer struct {
	// dialer holds the JujuDialer.
	dialer JujuDialer

	// mu is for protecting the map when retieving/adding two different
	// cached controller connections. We could in theory just have separate maps per
	// controller, but this works fine.
	mu sync.Mutex
	// sf to handle duplicates when attempting to insert a connection for the same controller.
	sf singleflight.Group

	// conns stores the connections.
	// TODO(ale8k): Use sync.Map and delete mux.
	conns map[string]api.Connection

	// cleanupIntervalTicker cleans up broken connections in the cache.
	cleanupIntervalTicker *time.Ticker
}

// NewCacheDialer returns a new cache dialer intended to wrap the base JujuDialer.
// It ensures that two routines do not connect independently and instead
// share the same connection PER controller.
//
// TODO(ale8k): Do we need this for models too?
func NewCacheDialer(d JujuDialer, cleanupInterval time.Duration) *cacheJujuDialer {
	cjd := &cacheJujuDialer{
		dialer:                d,
		cleanupIntervalTicker: time.NewTicker(cleanupInterval),
		conns:                 make(map[string]api.Connection),
	}
	go cjd.cleanupConnections()
	return cjd
}

// Dial works like so:
//
// It uses a singleflight for:
//   - Preventing duplicate cache inserts
//   - Allow only one routine to access the cache at a time
//
// It uses a map of connections for:
//   - Preventing the need to connect multiple times to the same controller
//
// It uses a mux for:
//   - Mux to protect the map (there are libs for this though, perhaps use one of those)
func (cjd *cacheJujuDialer) Dial(p DialParams) (api.Connection, error) {
	if p.ModelTag.Id() != "" {
		return cjd.dialer.Dial(p)
	}

	ctlUuid := p.ControllerTag.Id()

	v, err, _ := cjd.sf.Do(ctlUuid, func() (any, error) {
		cjd.mu.Lock()
		conn, exists := cjd.conns[ctlUuid]
		cjd.mu.Unlock()

		if exists && !conn.IsBroken() {
			return conn, nil
		}

		conn, err := cjd.dialer.Dial(p)
		if err != nil {
			return nil, err
		}

		cjd.mu.Lock()
		cjd.conns[ctlUuid] = conn
		cjd.mu.Unlock()

		return conn, nil
	})

	if err != nil {
		return nil, err
	}

	return v.(api.Connection), nil
}

// cleanupConnections checks for broken connections and:
//
// - Removes them from the cache map
// - Closes them
// - Forgets the controller from the singleflight
func (cd *cacheJujuDialer) cleanupConnections() {
	for range cd.cleanupIntervalTicker.C {
		cd.mu.Lock()
		for key, conn := range cd.conns {
			if conn.IsBroken() {
				conn.Close() // TODO(ale8k): Do we need this?
				cd.sf.Forget(conn.ControllerTag().Id())
				delete(cd.conns, key)
			}
		}
		cd.mu.Unlock()
	}
}
