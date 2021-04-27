// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/juju/names/v4"
	"golang.org/x/sync/singleflight"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

// CacheDialer wraps the given Dialer in a cache that will share controller
// connections between a number of operations.
func CacheDialer(d Dialer) Dialer {
	return &cacheDialer{
		dialer: d,
		conns:  make(map[string]cachedAPI),
	}
}

// A cacheDialer is a Dialer that caches connections so that the cost of
// establishing connections is shared by a number of operations attempting
// to contact a controller.
type cacheDialer struct {
	// dialer is used by the CacheDialer to make connections that are
	// not in the cache.
	dialer Dialer

	sfg   singleflight.Group
	mu    sync.Mutex
	conns map[string]cachedAPI
}

// Dial implements Dialer.Dial.
func (d *cacheDialer) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag) (API, error) {
	if mt.Id() != "" {
		// connections to models are rare, so we don't cache them.
		return d.dialer.Dial(ctx, ctl, mt)
	}
	rc := d.sfg.DoChan(ctl.Name, func() (interface{}, error) {
		return d.dial(ctx, ctl)
	})
	select {
	case r := <-rc:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(cachedAPI).Clone(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *cacheDialer) dial(ctx context.Context, ctl *dbmodel.Controller) (interface{}, error) {
	d.mu.Lock()
	capi, ok := d.conns[ctl.Name]
	if ok && !capi.IsBroken() {
		// connection is still good, return it.
		d.mu.Unlock()
		return capi, nil
	}
	if ok {
		// The connection must be broken, evict from the cache.
		delete(d.conns, ctl.Name)
	}
	d.mu.Unlock()
	if ok {
		// If the connection was broken, close it.
		capi.Close()
	}
	// We don't have a working connection to the controller, so dial one.
	api, err := d.dialer.Dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		return nil, err
	}
	capi = cachedAPI{
		API:      api,
		refCount: new(int64),
		closed:   new(uint32),
	}
	atomic.StoreInt64(capi.refCount, 1)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conns[ctl.Name] = capi
	return capi, nil
}

// Close implements io.Closer.
func (d *cacheDialer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var firstErr error
	for k, v := range d.conns {
		delete(d.conns, k)
		if err := v.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type cachedAPI struct {
	API

	// refCount is the number of open instances of the connection. When
	// refCount reaches 0 the underlying connection is closed.
	refCount *int64
	closed   *uint32
}

// Close implements API.Close()
func (a cachedAPI) Close() error {
	// Protect from closing the same clone multiple times.
	if !atomic.CompareAndSwapUint32(a.closed, 0, 1) {
		return nil
	}
	if atomic.AddInt64(a.refCount, -1) > 0 {
		return nil
	}
	// There are no references left, close the connection.
	return a.API.Close()
}

// Clone creates a clone of the cached API connection.
func (a cachedAPI) Clone() API {
	closed := new(uint32)
	atomic.AddInt64(a.refCount, 1)
	return cachedAPI{
		API:      a.API,
		refCount: a.refCount,
		closed:   closed,
	}
}
