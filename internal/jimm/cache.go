// Copyright 2024 Canonical.

package jimm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/canonical/jimm/v3/internal/dbmodel"
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
func (d *cacheDialer) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, requiredPermissions map[string]string) (API, error) {
	if mt.Id() != "" {
		// connections to models are rare, so we don't cache them.
		return d.dialer.Dial(ctx, ctl, mt, requiredPermissions)
	}
	rc := d.sfg.DoChan(ctl.Name, func() (interface{}, error) {
		return d.dial(ctx, ctl, requiredPermissions)
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

func (d *cacheDialer) dial(ctx context.Context, ctl *dbmodel.Controller, requiredPermissions map[string]string) (interface{}, error) {
	d.mu.Lock()
	capi, ok := d.conns[ctl.Name]
	if ok {
		if err := capi.Ping(ctx); err == nil {
			d.mu.Unlock()
			return capi, nil
		} else {
			zapctx.Warn(ctx, "cached connection failed", zap.Error(err))
			delete(d.conns, ctl.Name)
			capi.Close()
		}
	}
	d.mu.Unlock()

	// We don't have a working connection to the controller, so dial one.
	api, err := d.dialer.Dial(ctx, ctl, names.ModelTag{}, requiredPermissions)
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
