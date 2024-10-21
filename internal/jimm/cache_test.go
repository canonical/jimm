// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestCacheDialerDialError(t *testing.T) {
	c := qt.New(t)

	testError := errors.E("test error")
	testDialer := &jimmtest.Dialer{
		Err: testError,
	}
	dialer := jimm.CacheDialer(testDialer)
	ctl := dbmodel.Controller{
		Name: "test-controller",
	}
	_, err := dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
	c.Check(err, qt.Equals, testError)

	testAPI := jimmtest.API{
		SupportsCheckCredentialModels_: true,
	}
	testDialer.Err = nil
	testDialer.API = &testAPI
	api, err := dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
	c.Assert(err, qt.IsNil)
	c.Check(api.SupportsCheckCredentialModels(), qt.Equals, true)
}

func TestCacheDialerDialModel(t *testing.T) {
	c := qt.New(t)

	testAPI := jimmtest.API{
		SupportsCheckCredentialModels_: true,
	}
	testDialer := &jimmtest.Dialer{
		API: &testAPI,
	}
	dialer := jimm.CacheDialer(testDialer)
	ctl := dbmodel.Controller{
		Name: "test-controller",
	}
	mt := names.NewModelTag("00000002-0000-0000-0000-000000000001")
	api, err := dialer.Dial(context.Background(), &ctl, mt, nil)
	c.Assert(err, qt.IsNil)
	c.Check(api.SupportsCheckCredentialModels(), qt.Equals, true)

	testAPI2 := jimmtest.API{
		SupportsModelSummaryWatcher_: true,
	}
	testDialer.API = &testAPI2
	api, err = dialer.Dial(context.Background(), &ctl, mt, nil)
	c.Assert(err, qt.IsNil)
	c.Check(api.SupportsModelSummaryWatcher(), qt.Equals, true)
}

func TestCacheDialerConcurrentConnections(t *testing.T) {
	c := qt.New(t)

	testDialer := &countingDialer{
		dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				SupportsCheckCredentialModels_: true,
			},
		},
	}
	dialer := jimm.CacheDialer(testDialer)
	ctl := dbmodel.Controller{
		Name: "test-controller",
	}

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			api, err := dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
			c.Check(err, qt.IsNil)
			defer api.Close()
			c.Check(api.SupportsCheckCredentialModels(), qt.Equals, true)
			// close our API connection twice to ensure the
			// cache is robust to that.
			err = api.Close()
			c.Check(err, qt.IsNil)
		}()
	}
	wg.Wait()
	// Get a connection from the cache.
	api, err := dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
	c.Check(err, qt.IsNil)
	c.Check(api.SupportsCheckCredentialModels(), qt.Equals, true)
	err = api.Close()
	c.Check(err, qt.IsNil)

	c.Check(atomic.LoadInt64(&testDialer.count), qt.Equals, int64(1))
	c.Check(testDialer.dialer.(*jimmtest.Dialer).IsClosed(), qt.Equals, false)
	err = dialer.(io.Closer).Close()
	c.Check(err, qt.IsNil)
	c.Check(testDialer.dialer.(*jimmtest.Dialer).IsClosed(), qt.Equals, true)
}

func TestCacheDialerCloseBrokenConnection(t *testing.T) {
	c := qt.New(t)

	testAPI := closeCountingAPI{
		API: &jimmtest.API{
			// The cache assumes the a newly dialed
			// connection is good, so won't check for a
			// failed connection until retrieving it
			// from the cache.
			Ping_: func(context.Context) error {
				return errors.E("ping error")
			},
		},
	}
	testDialer := &countingDialer{
		dialer: &jimmtest.Dialer{
			API: &testAPI,
		},
	}
	dialer := jimm.CacheDialer(testDialer)
	ctl := dbmodel.Controller{
		Name: "test-controller",
	}

	api, err := dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
	c.Assert(err, qt.IsNil)
	err = api.Close()
	c.Assert(err, qt.IsNil)
	api2, err := dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
	c.Assert(err, qt.IsNil)
	err = api2.Close()
	c.Assert(err, qt.IsNil)

	c.Check(atomic.LoadInt64(&testDialer.count), qt.Equals, int64(2))
	c.Check(atomic.LoadInt64(&testAPI.count), qt.Equals, int64(1))
}

type countingDialer struct {
	dialer jimm.Dialer
	count  int64
}

func (d *countingDialer) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, requiredPermissions map[string]string) (jimm.API, error) {
	atomic.AddInt64(&d.count, 1)
	return d.dialer.Dial(ctx, ctl, mt, requiredPermissions)
}

type closeCountingAPI struct {
	jimm.API
	count int64
}

func (a *closeCountingAPI) Close() error {
	atomic.AddInt64(&a.count, 1)
	return a.API.Close()
}

func TestCacheDialerContextCanceled(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	doneC := make(chan struct{})
	dialer := jimm.CacheDialer(dialerFunc(func(context.Context, *dbmodel.Controller, names.ModelTag, map[string]string) (jimm.API, error) {
		cancel()
		<-doneC
		return nil, errors.E("dial error")
	}))
	ctl := dbmodel.Controller{
		UUID: jimmtest.ControllerUUID,
		Name: "test-controller",
	}
	api, err := dialer.Dial(ctx, &ctl, names.ModelTag{}, nil)
	c.Check(err, qt.Equals, context.Canceled)
	c.Check(api, qt.IsNil)
	close(doneC)
}

type dialerFunc func(context.Context, *dbmodel.Controller, names.ModelTag, map[string]string) (jimm.API, error)

func (f dialerFunc) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, requiredPermissions map[string]string) (jimm.API, error) {
	return f(ctx, ctl, mt, requiredPermissions)
}
