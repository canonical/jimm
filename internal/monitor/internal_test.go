// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	"github.com/juju/idmclient"
	corejujutesting "github.com/juju/juju/juju/testing"
	jujuwatcher "github.com/juju/juju/state/watcher"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type internalSuite struct {
	corejujutesting.JujuConnSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	jem    *jem.JEM

	// startTime holds the time that the testing clock is initially
	// set to.
	startTime time.Time

	// clock holds the mock clock used by the monitor package.
	clock *jujutesting.Clock
}

// We don't want to wait for the usual 5s poll interval.
func init() {
	jujuwatcher.Period = 50 * time.Millisecond
}

var _ = gc.Suite(&internalSuite{})

func (s *internalSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(
		jem.ServerParams{
			DB: s.Session.DB("jem"),
		},
		bakery.NewServiceParams{
			Location: "here",
		},
		idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	)
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = pool.JEM()

	// Set up the clock mockery.
	s.clock = jujutesting.NewClock(epoch)
	s.PatchValue(&Clock, s.clock)
}

func (s *internalSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

var epoch = parseTime("2016-01-01T12:00:00Z")

func (s *internalSuite) TestLeaseUpdater(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	}, &mongodoc.Model{})

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)

	m := &controllerMonitor{
		ctlPath:     ctlPath,
		leaseExpiry: expiry,
		jem:         jemShim{s.jem},
		ownerId:     "jem1",
	}
	done := make(chan error)
	go func() {
		done <- m.leaseUpdater()
	}()

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	// Wait for the lease to actually be renewed.
	var ctl *mongodoc.Controller
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.jem.Controller(ctlPath)
		c.Assert(err, gc.IsNil)
		if !ctl.MonitorLeaseExpiry.Equal(expiry) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("lease never acquired")
		}
	}
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), gc.DeepEquals, s.clock.Now().Add(leaseExpiryDuration))
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, "jem1")

	// Kill the monitor and wait for the updater to exit.
	m.tomb.Kill(nil)

	select {
	case err = <-done:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("lease updater never stopped")
	}
	c.Assert(err, gc.Equals, tomb.ErrDying)

	// Check that the lease has been dropped.
	s.assertLease(c, ctlPath, time.Time{}, "")
}

func (s *internalSuite) TestLeaseUpdaterWhenControllerRemoved(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	// Start the lease updater with no controller existing.
	m := &controllerMonitor{
		ctlPath:     ctlPath,
		leaseExpiry: epoch.Add(leaseExpiryDuration),
		jem:         jemShim{s.jem},
		ownerId:     "jem1",
	}
	defer m.tomb.Kill(nil)
	done := make(chan error)
	go func() {
		done <- m.leaseUpdater()
	}()

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	var err error
	select {
	case err = <-done:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("lease updater never stopped")
	}
	c.Assert(err, gc.ErrorMatches, `cannot renew lease on bob/foo: controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, errMonitoringStopped)
}

func (s *internalSuite) TestWatcher(c *gc.C) {
	// Add some entities to the admin model.
	f := factory.NewFactory(s.State)
	svc := f.MakeService(c, &factory.ServiceParams{
		Name: "wordpress",
	})
	f.MakeUnit(c, &factory.UnitParams{
		Service: svc,
	})
	f.MakeUnit(c, &factory.UnitParams{
		Service: svc,
	})
	modelSt := f.MakeModel(c, &factory.ModelParams{
		Name: "jem-somemodel",
	})
	defer modelSt.Close()

	// Add some JEM entities.
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)
	err := s.jem.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}, &mongodoc.Model{
		UUID: info.ModelTag.Id(),
	})
	c.Assert(err, gc.IsNil)

	// Make a JEM model that refers to the juju model.
	modelPath := params.EntityPath{"alice", "bar"}
	err = s.jem.AddModel(&mongodoc.Model{
		Path:       modelPath,
		Controller: ctlPath,
		UUID:       modelSt.ModelUUID(),
	})
	c.Assert(err, gc.IsNil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(s.jem)
	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(m.watcher)
	// Ensure we always wait for the watcher to stop, otherwise
	// it can use the JEM connection after the test returns, with
	// nasty results.
	defer func() {
		m.tomb.Kill(nil)
		err := m.tomb.Wait()
		c.Check(errgo.Cause(err), gc.Equals, errMonitoringStopped)
	}()

	// The watcher should set the model life and the
	// controller stats. We have two models, so we'll see
	// two set.
	waitEvent(c, jshim.modelLifeSet, "model life")
	waitEvent(c, jshim.modelLifeSet, "model life")
	waitEvent(c, jshim.controllerStatsSet, "controller stats")
	jshim.assertNoEvent(c)

	s.assertControllerStats(c, ctlPath, mongodoc.ControllerStats{
		ModelCount:   2,
		ServiceCount: 1,
		UnitCount:    2,
		MachineCount: 2,
	})
	s.assertModelLife(c, ctlPath, "alive")
	s.assertModelLife(c, modelPath, "alive")

	// Add another service and check that the service count is maintained.
	f.MakeService(c, &factory.ServiceParams{
		Name: "mysql",
	})

	waitEvent(c, jshim.controllerStatsSet, "controller stats")
	s.assertControllerStats(c, ctlPath, mongodoc.ControllerStats{
		ModelCount:   2,
		ServiceCount: 2,
		UnitCount:    2,
		MachineCount: 2,
	})

	stm, err := modelSt.Model()
	c.Assert(err, gc.IsNil)
	err = stm.Destroy()
	c.Assert(err, gc.IsNil)

	// Destroy the model and check that its life status changes.
	waitEvent(c, jshim.modelLifeSet, "model life")
	jshim.assertNoEvent(c)

	s.assertModelLife(c, modelPath, "dead")

	err = modelSt.RemoveAllModelDocs()
	c.Assert(err, gc.IsNil)

	// We won't see the model life set because it's still the same as before.
	waitEvent(c, jshim.controllerStatsSet, "controller stats")
	jshim.assertNoEvent(c)

	s.assertControllerStats(c, ctlPath, mongodoc.ControllerStats{
		ModelCount:   1,
		ServiceCount: 2,
		UnitCount:    2,
		MachineCount: 2,
	})
	s.assertModelLife(c, modelPath, "dead")
}

func (s *internalSuite) TestWatcherKilledWhileDialingAPI(c *gc.C) {
	info := s.APIInfo(c)
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.AddController(&mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    info.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
	}, &mongodoc.Model{
		UUID: "some-uuid",
	})
	c.Assert(err, gc.IsNil)

	openCh := make(chan struct{})

	// Start the watcher.
	jshim := jemShimWithAPIOpener{
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			openCh <- struct{}{}
			<-openCh
			return nil, errgo.New("ignored error")
		},
		jemInterface: jemShim{s.jem},
	}
	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(m.watcher)
	// Ensure we always wait for the watcher to stop, otherwise
	// it can use the JEM connection after the test returns, with
	// nasty results.
	defer func() {
		m.tomb.Kill(nil)
		err := m.tomb.Wait()
		c.Check(errgo.Cause(err), gc.Equals, errMonitoringStopped)
	}()

	// Wait for the API to be opened.
	waitEvent(c, openCh, "open API")

	// Kill the watcher tomb and check that it dies even
	// though the API open is still going.
	m.tomb.Kill(nil)
	waitEvent(c, m.tomb.Dead(), "watcher termination")
	err = m.tomb.Wait()
	c.Check(errgo.Cause(err), gc.Equals, errMonitoringStopped)

	// Let the asynchronously started API opener terminate.
	openCh <- struct{}{}
}

func (s *internalSuite) TestWatcherDialAPIError(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.AddController(&mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujutesting.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
	}, &mongodoc.Model{
		UUID: "some-uuid",
	})
	c.Assert(err, gc.IsNil)

	apiErrorCh := make(chan error)

	// Start the watcher.
	jshim := jemShimWithAPIOpener{
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			return nil, <-apiErrorCh
		},
		jemInterface: jemShim{s.jem},
	}
	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(m.watcher)
	// Ensure we always wait for the watcher to stop, otherwise
	// it can use the JEM connection after the test returns, with
	// nasty results.
	defer worker.Stop(m)

	// First send a jem.ErrAPIConnection error. This should cause the
	// watcher to pause for a while and retry.
	select {
	case apiErrorCh <- errgo.WithCausef(nil, jem.ErrAPIConnection, ""):
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for API connection")
	}

	// Check that the watcher doesn't try again immediately.
	select {
	case apiErrorCh <- errgo.New("oh no you don't"):
		c.Fatalf("watcher did not wait before retrying")
	case <-time.After(jujutesting.ShortWait):
	}

	// Advance the time until past the retry time.
	s.clock.Advance(apiConnectRetryDuration)

	select {
	case apiErrorCh <- errgo.New("fatal error"):
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for retried API connection")
	}

	// This non-ErrAPIConnection error should cause the
	// watcher to die.
	waitEvent(c, m.tomb.Dead(), "watcher dead")

	c.Assert(m.tomb.Err(), gc.ErrorMatches, "cannot dial API for controller bob/foo: fatal error")
}

// TestControllerMonitor tests that the controllerMonitor can be run with both the
// lease updater and the watcher in place.
func (s *internalSuite) TestControllerMonitor(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)
	err := s.jem.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}, &mongodoc.Model{
		UUID: info.ModelTag.Id(),
	})
	c.Assert(err, gc.IsNil)

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)

	jshim := newJEMShimWithUpdateNotify(s.jem)
	m := newControllerMonitor(controllerMonitorParams{
		ctlPath:     ctlPath,
		jem:         jshim,
		ownerId:     "jem1",
		leaseExpiry: expiry,
	})
	defer worker.Stop(m)

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	waitEvent(c, jshim.controllerStatsSet, "controller stats")
	waitEvent(c, jshim.modelLifeSet, "model life")
	waitEvent(c, jshim.leaseAcquired, "lease acquisition")
	jshim.assertNoEvent(c)

	s.assertControllerStats(c, ctlPath, mongodoc.ControllerStats{
		ModelCount: 1,
	})
	s.assertModelLife(c, ctlPath, "alive")
	s.assertLease(c, ctlPath, epoch.Add(leaseExpiryDuration*5/6+leaseExpiryDuration), "jem1")

	err = worker.Stop(m)
	c.Assert(err, gc.IsNil)

	// Check that the lease has been dropped.
	s.assertLease(c, ctlPath, time.Time{}, "")
}

func (s *internalSuite) assertControllerStats(c *gc.C, ctlPath params.EntityPath, stats mongodoc.ControllerStats) {
	ctlDoc, err := s.jem.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctlDoc.Stats, jc.DeepEquals, stats)
}

func (s *internalSuite) assertModelLife(c *gc.C, modelPath params.EntityPath, life string) {
	modelDoc, err := s.jem.Model(modelPath)
	c.Assert(err, gc.IsNil)
	c.Assert(modelDoc.Life, gc.Equals, life)
}

func (s *internalSuite) assertLease(c *gc.C, ctlPath params.EntityPath, t time.Time, owner string) {
	ctl, err := s.jem.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, owner)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func waitEvent(c *gc.C, ch <-chan struct{}, what string) {
	select {
	case <-ch:
	case <-time.After(jujutesting.LongWait):
		select {}
		c.Fatalf("timed out waiting for %s", what)
	}
}

func assertNoEvent(c *gc.C, ch <-chan struct{}, what string) {
	select {
	case <-ch:
		c.Fatalf("unexpected event received: %v", what)
	case <-time.After(jujutesting.ShortWait):
	}
}
