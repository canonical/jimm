// Copyright 2016 Canonical Ltd.

package monitor

import (
	"fmt"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	corejujutesting "github.com/juju/juju/juju/testing"
	jujuwatcher "github.com/juju/juju/state/watcher"
	jujujujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/tomb.v2"

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
	pool, err := jem.NewPool(jem.Params{
		DB: s.Session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
		ControllerAdmin: "controller-admin",
	})
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
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})

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
	for a := jujujujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.jem.DB.Controller(ctlPath)
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
	case <-time.After(jujujujutesting.LongWait):
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
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("lease updater never stopped")
	}
	c.Assert(err, gc.ErrorMatches, `cannot renew lease on bob/foo: controller has been removed`)
	c.Assert(errgo.Cause(err), gc.Equals, errControllerRemoved)
	c.Assert(err, jc.Satisfies, isMonitoringStoppedError)
}

func (s *internalSuite) TestWatcher(c *gc.C) {
	// Add some entities to the admin model.
	f := factory.NewFactory(s.State)
	svc := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: svc,
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: svc,
	})
	modelSt := f.MakeModel(c, &factory.ModelParams{
		Name: "jem-somemodel",
	})
	defer modelSt.Close()

	// Add some JEM entities.
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})
	c.Assert(err, gc.IsNil)

	// Make a JEM model that refers to the juju model.
	modelPath := params.EntityPath{"alice", "bar"}
	err = s.jem.DB.AddModel(&mongodoc.Model{
		Path:       modelPath,
		Controller: ctlPath,
		UUID:       modelSt.ModelUUID(),
	})
	c.Assert(err, gc.IsNil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.jem})
	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(m.watcher)
	defer cleanStop(c, m)

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
	s.assertModelLife(c, modelPath, "alive")

	// Add another service and check that the service count is maintained.
	f.MakeApplication(c, &factory.ApplicationParams{
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
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    info.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
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
	defer cleanStop(c, m)

	// Wait for the API to be opened.
	waitEvent(c, openCh, "open API")

	// Kill the watcher tomb and check that it dies even
	// though the API open is still going.
	m.tomb.Kill(nil)
	waitEvent(c, m.tomb.Dead(), "watcher termination")
	err = m.tomb.Wait()
	c.Check(err, gc.Equals, nil)

	// Let the asynchronously started API opener terminate.
	openCh <- struct{}{}
}

func (s *internalSuite) TestWatcherDialAPIError(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
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
	defer worker.Stop(m)

	// First send a jem.ErrAPIConnection error. This should cause the
	// watcher to pause for a while and retry.
	select {
	case apiErrorCh <- errgo.WithCausef(nil, jem.ErrAPIConnection, "faked api connection error"):
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for API connection")
	}

	// Check that the watcher doesn't try again immediately.
	select {
	case apiErrorCh <- errgo.New("oh no you don't"):
		c.Fatalf("watcher did not wait before retrying")
	case <-time.After(jujujujutesting.ShortWait):
	}

	// Check that the controller is marked as unavailable.
	ctl, err := s.jem.DB.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), gc.DeepEquals, s.clock.Now().UTC())

	// Advance the time until past the retry time.
	s.clock.Advance(apiConnectRetryDuration)

	select {
	case apiErrorCh <- errgo.New("fatal error"):
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for retried API connection")
	}

	// This non-ErrAPIConnection error should cause the
	// watcher to die.
	waitEvent(c, m.tomb.Dead(), "watcher dead")

	c.Assert(m.Wait(), gc.ErrorMatches, "cannot dial API for controller bob/foo: fatal error")
}

func (s *internalSuite) TestWatcherMarksControllerAvailable(c *gc.C) {
	jshim := newJEMShimInMemory()
	apiShims := newJujuAPIShims()
	defer apiShims.CheckAllClosed(c)
	jshim1 := newJEMShimWithUpdateNotify(jemShimWithAPIOpener{
		jemInterface: jshim,
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			return apiShims.newJujuAPIShim(nil), nil
		},
	})
	// Create a controller
	ctlPath := params.EntityPath{"bob", "foo"}
	addFakeController(jshim, ctlPath)
	err := jshim1.SetControllerUnavailableAt(ctlPath, s.clock.Now())
	c.Assert(err, gc.IsNil)

	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(m.watcher)
	defer worker.Stop(m)

	waitEvent(c, jshim1.controllerAvailabilitySet, "controller available")
}

// TestControllerMonitor tests that the controllerMonitor can be run with both the
// lease updater and the watcher in place.
func (s *internalSuite) TestControllerMonitor(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})
	c.Assert(err, gc.IsNil)

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)

	jshim := newJEMShimWithUpdateNotify(jemShim{s.jem})
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
	s.assertLease(c, ctlPath, epoch.Add(leaseExpiryDuration*5/6+leaseExpiryDuration), "jem1")

	err = worker.Stop(m)
	c.Assert(err, gc.IsNil)

	// Check that the lease has been dropped.
	s.assertLease(c, ctlPath, time.Time{}, "")
}

func (s *internalSuite) TestControllerMonitorDiesWithMonitoringStoppedErrorWhenControllerIsRemoved(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})
	c.Assert(err, gc.IsNil)

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)
	err = s.jem.DB.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)

	m := newControllerMonitor(controllerMonitorParams{
		ctlPath:     ctlPath,
		jem:         jemShim{s.jem},
		ownerId:     "jem1",
		leaseExpiry: expiry,
	})
	defer worker.Stop(m)
	waitEvent(c, m.Dead(), "monitor dead")
	err = m.Wait()
	c.Assert(err, jc.Satisfies, isMonitoringStoppedError)
	c.Assert(errgo.Cause(err), gc.Equals, errControllerRemoved)
}

func (s *internalSuite) TestAllMonitorSingleControllerWithAPIError(c *gc.C) {
	jshim := newJEMShimInMemory()
	addFakeController(jshim, params.EntityPath{"bob", "foo"})
	m := newAllMonitor(jshim, "jem1")
	waitEvent(c, m.tomb.Dead(), "monitor dead")
	c.Assert(m.tomb.Err(), gc.ErrorMatches, `cannot dial API for controller bob/foo: jemShimInMemory doesn't implement OpenAPI`)
	c.Assert(jshim.refCount, gc.Equals, 0)

	// Check that the lease has been dropped.
	ctl := jshim.controllers[params.EntityPath{"bob", "foo"}]
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, "")
	c.Assert(ctl.MonitorLeaseExpiry, jc.Satisfies, time.Time.IsZero)
}

func (s *internalSuite) TestAllMonitorMultiControllersWithAPIError(c *gc.C) {
	jshim := newJEMShimInMemory()

	ctlPath := func(i int) params.EntityPath {
		return params.EntityPath{"bob", params.Name(fmt.Sprintf("x%d", i))}
	}
	const ncontrollers = 3
	for i := 0; i < ncontrollers; i++ {
		addFakeController(jshim, ctlPath(i))
	}

	opened := make(map[params.EntityPath]bool)
	openCh := make(chan params.EntityPath)
	openReply := make(chan error)
	m := newAllMonitor(jemShimWithAPIOpener{
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			openCh <- path
			return nil, <-openReply
		},
		jemInterface: jshim,
	}, "jem1")
	defer worker.Stop(m)

	for i := 0; i < ncontrollers; i++ {
		select {
		case p := <-openCh:
			if opened[p] {
				c.Fatalf("controller %v opened twice", p)
			}
			opened[p] = true
		case <-time.After(jujujujutesting.LongWait):
			c.Fatalf("timed out waiting for API to be opened")
		}
	}
	// Check that all the controllers are accounted for.
	for i := 0; i < ncontrollers; i++ {
		p := ctlPath(i)
		c.Assert(opened[p], gc.Equals, true, gc.Commentf("controller %v", p))
	}
	// All the controllers are now blocked trying to open the
	// API. Send one of them an error. The allMonitor should
	// die with that error, leaving the other API opens in progress.
	openReply <- errgo.New("some error")

	waitEvent(c, m.tomb.Dead(), "monitor dead")
	c.Assert(m.tomb.Err(), gc.ErrorMatches, `cannot dial API for controller bob/x[0-9]+: some error`)

	// Check that all leases have been dropped.
	for i := 0; i < ncontrollers; i++ {
		p := ctlPath(i)
		ctl := jshim.controllers[p]
		c.Assert(ctl.MonitorLeaseOwner, gc.Equals, "", gc.Commentf("controller %v", p))
		c.Assert(ctl.MonitorLeaseExpiry, jc.Satisfies, time.Time.IsZero, gc.Commentf("controller %v", p))
	}

	// Send errors to the other controller API opens.
	for i := 0; i < ncontrollers-1; i++ {
		openReply <- errgo.New("another error")
	}
}

func (s *internalSuite) TestAllMonitorMultiControllerMultipleLeases(c *gc.C) {
	jshim := newJEMShimInMemory()
	apiShims := newJujuAPIShims()
	defer apiShims.CheckAllClosed(c)
	jshim1 := jemShimWithAPIOpener{
		jemInterface: jshim,
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			return apiShims.newJujuAPIShim(nil), nil
		},
	}
	type leaseAcquisition struct {
		path  params.EntityPath
		owner string
	}
	leaseAcquired := make(chan leaseAcquisition, 10)
	jshim2 := jemShimWithMonitorLeaseAcquirer{
		jemInterface: jshim1,
		acquireMonitorLease: func(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
			t, err := jshim1.AcquireMonitorLease(ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
			if err != nil {
				return time.Time{}, err
			}
			leaseAcquired <- leaseAcquisition{
				path:  ctlPath,
				owner: newOwner,
			}
			return t, nil
		},
	}

	// Create a controller
	addFakeController(jshim, params.EntityPath{"bob", "foo"})

	// Start the first monitor.
	m1 := newAllMonitor(jshim2, "jem1")
	defer worker.Stop(m1)

	// Wait for it to take out the first lease.
	select {
	case a := <-leaseAcquired:
		c.Assert(a.path.String(), gc.Equals, "bob/foo")
		c.Assert(a.owner, gc.Equals, "jem1")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for lease to be acquired")
	}

	// Wait for it to sleep after starting the monitors, which means
	// we know that it won't be acquiring any more controllers
	// until its lease acquire interval is finished, so when we
	// start the second monitor below, it'll be guaranteed to
	// acquire the second controller.
	<-s.clock.Alarms()

	// Create another controller
	addFakeController(jshim, params.EntityPath{"bob", "bar"})

	// Start another monitor. We can use the same JEM instance.
	m2 := newAllMonitor(jshim2, "jem2")
	defer worker.Stop(m2)

	// Wait for it to take out the second lease.
	select {
	case a := <-leaseAcquired:
		c.Assert(a.path.String(), gc.Equals, "bob/bar")
		c.Assert(a.owner, gc.Equals, "jem2")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for lease to be acquired")
	}

	// Advance the clock until both monitors will need to have
	// their leases renewed.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	renewed := make(map[string]string)
	timeout := time.After(jujujujutesting.LongWait)
	for i := 0; i < 2; i++ {
		select {
		case a := <-leaseAcquired:
			renewed[a.path.String()] = a.owner
		case <-timeout:
			c.Fatalf("timed out waiting for lease renewal %d", i+1)
		}
	}
	select {
	case p := <-leaseAcquired:
		c.Fatalf("unexpected lease acquired %v", p)
	case <-time.After(jujujujutesting.ShortWait):
	}
	// Both leases should still have the same owners.
	c.Assert(renewed, jc.DeepEquals, map[string]string{
		"bob/foo": "jem1",
		"bob/bar": "jem2",
	})

	// Kill one monitor and wait for the old one to take on the new lease.
	err := worker.Stop(m2)
	c.Assert(err, gc.IsNil)

	// It should drop its lease when stopped.
	select {
	case p := <-leaseAcquired:
		c.Assert(p.path.String(), gc.Equals, "bob/bar")
		c.Assert(p.owner, gc.Equals, "")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for lease drop")
	}

	// After leaseAcquireInterval, the worker should take notice.
	s.clock.Advance(leaseAcquireInterval)

	select {
	case p := <-leaseAcquired:
		c.Assert(p.path.String(), gc.Equals, "bob/bar")
		c.Assert(p.owner, gc.Equals, "jem1")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for lease acquisition")
	}
}

func (s *internalSuite) TestAllMonitorWithRaceOnLeaseAcquisition(c *gc.C) {
	jshim := jemShim{s.jem}

	// Fake the monitor lease acquiry so that it lets us know
	// when a monitor is trying to acquire the lease and
	// so we can let it proceed when we want to.
	wantLease := make(chan struct{}, 10)
	goforitLease := make(chan struct{})
	jshim1 := jemShimWithMonitorLeaseAcquirer{
		jemInterface: jshim,
		acquireMonitorLease: func(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
			wantLease <- struct{}{}
			<-goforitLease
			return jshim.AcquireMonitorLease(ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
		},
	}

	// Fake the API open call so that we know the number
	// of times the API has been opened. If only one monitor
	// acquires the lease, this should be only 1 (note that
	// the monitor sleeps before retrying the API open).
	apiOpened := make(chan struct{})
	jshim2 := jemShimWithAPIOpener{
		jemInterface: jshim1,
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			apiOpened <- struct{}{}
			<-apiOpened
			return nil, errgo.New("no API connections in this test")
		},
	}
	ctlPath := params.EntityPath{"bob", "foo"}
	//	info := s.APIInfo(c)
	err := s.jem.DB.AddController(&mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
	})
	c.Assert(err, gc.IsNil)

	// On your marks...
	m1 := newAllMonitor(jshim2, "jem1")
	defer worker.Stop(m1)

	m2 := newAllMonitor(jshim2, "jem2")
	defer worker.Stop(m2)

	// ... get set ...
	timeout := time.After(jujujujutesting.LongWait)
	for i := 0; i < 2; i++ {
		select {
		case <-wantLease:
		case <-timeout:
			c.Fatalf("timeout waiting for both monitors to try acquiring lease")
		}
	}

	// ... go!
	close(goforitLease)

	// Wait for both monitors to go to sleep after the race is over.
	timeout = time.After(jujujujutesting.LongWait)
	for i := 0; i < 2; i++ {
		select {
		case <-s.clock.Alarms():
		case <-timeout:
			c.Fatalf("timeout waiting for both monitors to sleep")
		}
	}

	// Assert that the API is opened only once.
	waitEvent(c, apiOpened, "api open")
	assertNoEvent(c, apiOpened, "api open")

	// Sanity check that the lease is actually held by one of the two monitors.
	ctl, err := s.jem.DB.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	if ctl.MonitorLeaseExpiry.IsZero() {
		c.Errorf("monitor lease not held")
	}
	if ctl.MonitorLeaseOwner != "jem1" && ctl.MonitorLeaseOwner != "jem2" {
		c.Errorf("unexpected monitor owner %q", ctl.MonitorLeaseOwner)
	}
	apiOpened <- struct{}{}
}

func (s *internalSuite) TestAllMonitorReusesOwnLease(c *gc.C) {
	jshim := newJEMShimInMemory()
	ctlPath := params.EntityPath{"bob", "foo"}
	addFakeController(jshim, ctlPath)

	openedAPI := make(chan params.EntityPath, 10)
	jshim1 := jemShimWithAPIOpener{
		jemInterface: jshim,
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			openedAPI <- path
			// Return an ErrAPIConnection error so that the
			// monitor will retry rather than tearing down
			// the connection, so the lease enquiry at the
			// end of this test won't be racing with the allMonitor
			// terminating.
			return nil, errgo.WithCausef(nil, jem.ErrAPIConnection, "no API connection in this test")
		},
	}

	expiry, err := jshim.AcquireMonitorLease(ctlPath, time.Time{}, "", epoch.Add(leaseExpiryDuration), "jem1")
	c.Assert(err, gc.IsNil)

	m1 := newAllMonitor(jshim1, "jem1")
	defer worker.Stop(m1)

	select {
	case p := <-openedAPI:
		c.Check(p, gc.Equals, ctlPath)
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for API open")
	}
	ctl := jshim.controllers[ctlPath]
	c.Assert(ctl, gc.NotNil)
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, "jem1")
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), jc.DeepEquals, expiry.UTC())
}

func (s *internalSuite) assertControllerStats(c *gc.C, ctlPath params.EntityPath, stats mongodoc.ControllerStats) {
	ctlDoc, err := s.jem.DB.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctlDoc.Stats, jc.DeepEquals, stats)
}

func (s *internalSuite) assertModelLife(c *gc.C, modelPath params.EntityPath, life string) {
	modelDoc, err := s.jem.DB.Model(modelPath)
	c.Assert(err, gc.IsNil)
	c.Assert(modelDoc.Life, gc.Equals, life)
}

func (s *internalSuite) assertLease(c *gc.C, ctlPath params.EntityPath, t time.Time, owner string) {
	ctl, err := s.jem.DB.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, owner)
}

func addFakeController(jshim *jemShimInMemory, path params.EntityPath) {
	jshim.AddController(&mongodoc.Controller{
		Path:      path,
		UUID:      fmt.Sprintf("some-uuid-%s", path),
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
	})
	jshim.AddModel(&mongodoc.Model{
		Path:       path,
		Controller: path,
	})
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
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for %s", what)
	}
}

func assertNoEvent(c *gc.C, ch <-chan struct{}, what string) {
	select {
	case <-ch:
		c.Fatalf("unexpected event received: %v", what)
	case <-time.After(jujujujutesting.ShortWait):
	}
}

func cleanStop(c *gc.C, w worker.Worker) {
	err := worker.Stop(w)
	c.Check(err, gc.IsNil)
}
