// Copyright 2016 Canonical Ltd.

package monitor

import (
	"fmt"
	"time"

	"github.com/juju/idmclient/idmtest"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	jujuwatcher "github.com/juju/juju/state/watcher"
	jujujujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemtest"
	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type internalSuite struct {
	jemtest.JujuConnSuite
	idmSrv      *idmtest.Server
	sessionPool *mgosession.Pool
	pool        *jem.Pool
	jem         *jem.JEM

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
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 1)
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		SessionPool:     s.sessionPool,
		DB:              s.Session.DB("jem"),
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = pool.JEM(context.TODO())

	// Set up the clock mockery.
	s.clock = jujutesting.NewClock(epoch)
	s.PatchValue(&Clock, s.clock)
}

func (s *internalSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

var epoch = parseTime("2016-01-01T12:00:00Z")
var testContext = context.Background()

func (s *internalSuite) TestLeaseUpdater(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(testContext, jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)

	m := &controllerMonitor{
		ctlPath:     ctlPath,
		leaseExpiry: expiry,
		jem:         jemShim{s.jem},
		ownerId:     "jem1",
	}
	done := make(chan error)
	go func() {
		done <- m.leaseUpdater(testContext)
	}()

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	// Wait for the lease to actually be renewed.
	var ctl *mongodoc.Controller
	for a := jujujujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.jem.DB.Controller(testContext, ctlPath)
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
		done <- m.leaseUpdater(testContext)
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
	// Add a couple of models and applications with units to watch.
	model1State := newModel(c, s.State, "model1")
	newApplication(c, model1State, "model1-app", 2)
	defer model1State.Close()

	model2State := newModel(c, s.State, "model2")
	newApplication(c, model2State, "model1-app", 2)
	// Add a co-hosted unit so that we can see a different between units and machines.
	addUnitOnMachine(c, model2State, "model1-app", "0")
	defer model2State.Close()

	ctlPath := params.EntityPath{"bob", "foo"}
	s.addJEMController(c, ctlPath)

	// Add the JEM model entries
	model1Path := params.EntityPath{"bob", "model1"}
	model2Path := params.EntityPath{"bob", "model2"}
	err := s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path:       model1Path,
		Controller: ctlPath,
		UUID:       model1State.ModelUUID(),
	})
	c.Assert(err, gc.IsNil)
	err = s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path:       model2Path,
		Controller: ctlPath,
		UUID:       model2State.ModelUUID(),
	})
	c.Assert(err, gc.IsNil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.jem})
	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
	defer cleanStop(c, m)

	type allStats struct {
		stats          mongodoc.ControllerStats
		model1, model2 modelStats
	}
	getAllStats := func() interface{} {
		return allStats{
			stats:  s.controllerStats(c, ctlPath),
			model1: s.modelStats(c, model1Path),
			model2: s.modelStats(c, model2Path),
		}
	}
	// The watcher should set the model life and the
	// controller stats. We have three models (the controller
	// has a model too, even though it's not in JEM), so we'll see
	// three set.

	jshim.await(c, getAllStats, allStats{
		stats: mongodoc.ControllerStats{
			ModelCount:   3,
			ServiceCount: 2,
			UnitCount:    5,
			MachineCount: 4,
		},
		model1: modelStats{
			life:             "alive",
			unitCount:        2,
			machineCount:     2,
			applicationCount: 1,
		},
		model2: modelStats{
			life:             "alive",
			unitCount:        3,
			machineCount:     2,
			applicationCount: 1,
		},
	})

	c.Logf("making model2-app2")

	// Add another application and check that the service count and unit counts
	// are maintained.
	newApplication(c, model2State, "model2-app2", 2)

	jshim.await(c, getAllStats, allStats{
		stats: mongodoc.ControllerStats{
			ModelCount:   3,
			ServiceCount: 3,
			UnitCount:    7,
			MachineCount: 6,
		},
		model1: modelStats{
			life:             "alive",
			unitCount:        2,
			machineCount:     2,
			applicationCount: 1,
		},
		model2: modelStats{
			life:             "alive",
			unitCount:        5,
			machineCount:     4,
			applicationCount: 2,
		},
	})

	// Destroy model1 and check that its life status moves to dying, but the rest stays the same.
	model1, err := model1State.Model()
	c.Assert(err, gc.IsNil)
	err = model1.Destroy()
	c.Assert(err, gc.IsNil)

	jshim.await(c, getAllStats, allStats{
		stats: mongodoc.ControllerStats{
			ModelCount:   3,
			ServiceCount: 3,
			UnitCount:    7,
			MachineCount: 6,
		},
		model1: modelStats{
			life:             "dying",
			unitCount:        2,
			machineCount:     2,
			applicationCount: 1,
		},
		model2: modelStats{
			life:             "alive",
			unitCount:        5,
			machineCount:     4,
			applicationCount: 2,
		},
	})

	// Destroy a model and check that the model life, model, service and unit counts
	// are maintained correctly.
	removeModel(c, model1State)

	jshim.await(c, getAllStats, allStats{
		stats: mongodoc.ControllerStats{
			ModelCount:   2,
			ServiceCount: 2,
			UnitCount:    5,
			MachineCount: 4,
		},
		model1: modelStats{
			life: "dead",
		},
		model2: modelStats{
			life:             "alive",
			unitCount:        5,
			machineCount:     4,
			applicationCount: 2,
		},
	})
}

func (s *internalSuite) TestWatcherUpdatesMachineInfo(c *gc.C) {
	// Add a couple of models and applications with units to watch.
	modelState := newModel(c, s.State, "model")
	newApplication(c, modelState, "model-app", 1)
	defer modelState.Close()

	ctlPath := params.EntityPath{"bob", "foo"}
	s.addJEMController(c, ctlPath)

	// Add the JEM model entries
	modelPath := params.EntityPath{"bob", "model"}
	err := s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path:       modelPath,
		Controller: ctlPath,
		UUID:       modelState.ModelUUID(),
	})
	c.Assert(err, gc.IsNil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.jem})
	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
	defer cleanStop(c, m)

	// Wait for just a subset of the entire information so that
	// we don't depend on loads of unrelated details - we
	// care that the information is updated, not the specifics of
	// what all the details are.
	type machineInfo struct {
		modelUUID string
		id        string
		life      multiwatcher.Life
	}

	getMachineInfo := func() interface{} {
		ms, err := s.jem.DB.MachinesForModel(testContext, modelState.ModelUUID())
		c.Assert(err, gc.IsNil)
		infos := make([]machineInfo, len(ms))
		for i, m := range ms {
			infos[i] = machineInfo{
				modelUUID: m.Info.ModelUUID,
				id:        m.Info.Id,
				life:      m.Info.Life,
			}
		}
		return infos
	}
	jshim.await(c, getMachineInfo, []machineInfo{{
		modelUUID: modelState.ModelUUID(),
		id:        "0",
		life:      "alive",
	}})
}

func removeModel(c *gc.C, st *state.State) {
	apps, err := st.AllApplications()
	c.Assert(err, gc.IsNil)
	for _, app := range apps {
		units, err := app.AllUnits()
		c.Assert(err, gc.IsNil)
		for _, unit := range units {
			err := unit.Destroy()
			c.Assert(err, gc.IsNil)
			err = unit.EnsureDead()
			c.Assert(err, gc.IsNil)
			err = unit.Remove()
			c.Assert(err, gc.IsNil)
		}
		err = app.Destroy()
		c.Assert(err, gc.IsNil)
	}
	machines, err := st.AllMachines()
	c.Assert(err, gc.IsNil)
	for _, machine := range machines {
		err = machine.Destroy()
		c.Assert(err, gc.IsNil)
		err = machine.EnsureDead()
		c.Assert(err, gc.IsNil)
		err = machine.Remove()
		c.Assert(err, gc.IsNil)
	}
	model, err := st.Model()
	c.Assert(err, gc.IsNil)
	err = model.Destroy()
	c.Assert(err, gc.IsNil)
	err = st.ProcessDyingModel()
	c.Assert(err, gc.IsNil)
	err = st.RemoveAllModelDocs()
	c.Assert(err, gc.IsNil)
}

func (s *internalSuite) TestWatcherKilledWhileDialingAPI(c *gc.C) {
	info := s.APIInfo(c)
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    info.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
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
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
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
	err := s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
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
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
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
	ctl, err := s.jem.DB.Controller(testContext, ctlPath)
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
	err := jshim1.SetControllerUnavailableAt(testContext, ctlPath, s.clock.Now())
	c.Assert(err, gc.IsNil)

	m := &controllerMonitor{
		ctlPath: ctlPath,
		jem:     jshim1,
		ownerId: "jem1",
	}
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
	defer worker.Stop(m)

	unavailableSince := func() interface{} {
		return jshim.controller(ctlPath).UnavailableSince
	}
	jshim1.await(c, unavailableSince, time.Time{})
}

// TestControllerMonitor tests that the controllerMonitor can be run with both the
// lease updater and the watcher in place.
func (s *internalSuite) TestControllerMonitor(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.IsNil)

	err = s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})
	c.Assert(err, gc.IsNil)

	err = s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path:       ctlPath,
		Controller: ctlPath,
		UUID:       info.ModelTag.Id(),
	})

	c.Assert(err, gc.IsNil)

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(testContext, jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)

	jshim := newJEMShimWithUpdateNotify(jemShim{s.jem})
	m := newControllerMonitor(context.TODO(), controllerMonitorParams{
		ctlPath:     ctlPath,
		jem:         jshim,
		ownerId:     "jem1",
		leaseExpiry: expiry,
	})
	defer worker.Stop(m)

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	type statsLifeLease struct {
		Stats       mongodoc.ControllerStats
		ModelLife   string
		LeaseExpiry time.Time
		LeaseOwner  string
	}
	getInfo := func() interface{} {
		ctl, err := s.jem.DB.Controller(testContext, ctlPath)
		c.Assert(err, gc.IsNil)
		return statsLifeLease{
			Stats:       ctl.Stats,
			ModelLife:   s.modelLife(c, ctlPath),
			LeaseExpiry: ctl.MonitorLeaseExpiry,
			LeaseOwner:  ctl.MonitorLeaseOwner,
		}
	}

	jshim.await(c, getInfo, statsLifeLease{
		Stats: mongodoc.ControllerStats{
			ModelCount: 1,
		},
		ModelLife:   "alive",
		LeaseExpiry: epoch.Add(leaseExpiryDuration*5/6 + leaseExpiryDuration),
		LeaseOwner:  "jem1",
	})

	err = worker.Stop(m)
	c.Assert(err, gc.IsNil)

	// Check that the lease has been dropped.
	s.assertLease(c, ctlPath, time.Time{}, "")
}

func (s *internalSuite) TestControllerMonitorDiesWithMonitoringStoppedErrorWhenControllerIsRemoved(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.IsNil)

	err = s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})

	c.Assert(err, gc.IsNil)

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(testContext, jemShim{s.jem}, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)
	err = s.jem.DB.DeleteController(context.TODO(), ctlPath)
	c.Assert(err, gc.IsNil)
	m := newControllerMonitor(context.TODO(), controllerMonitorParams{
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
	m := newAllMonitor(context.TODO(), jshim, "jem1")
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
	m := newAllMonitor(
		context.TODO(),
		jemShimWithAPIOpener{
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
		acquireMonitorLease: func(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
			t, err := jshim1.AcquireMonitorLease(ctx, ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
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
	m1 := newAllMonitor(context.TODO(), jshim2, "jem1")
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
	m2 := newAllMonitor(context.TODO(), jshim2, "jem2")
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
		acquireMonitorLease: func(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
			wantLease <- struct{}{}
			<-goforitLease
			return jshim.AcquireMonitorLease(ctx, ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
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
	err := s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
	})

	c.Assert(err, gc.IsNil)

	// On your marks...
	m1 := newAllMonitor(context.TODO(), jshim2, "jem1")
	defer worker.Stop(m1)

	m2 := newAllMonitor(context.TODO(), jshim2, "jem2")
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
	ctl, err := s.jem.DB.Controller(testContext, ctlPath)
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

	expiry, err := jshim.AcquireMonitorLease(testContext, ctlPath, time.Time{}, "", epoch.Add(leaseExpiryDuration), "jem1")
	c.Assert(err, gc.IsNil)

	m1 := newAllMonitor(context.TODO(), jshim1, "jem1")
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

func (s *internalSuite) controllerStats(c *gc.C, ctlPath params.EntityPath) mongodoc.ControllerStats {
	ctlDoc, err := s.jem.DB.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	return ctlDoc.Stats
}

// modelStats holds the aspects of a model updated by the monitor.
type modelStats struct {
	life                                      string
	unitCount, machineCount, applicationCount int
}

func (s *internalSuite) modelStats(c *gc.C, modelPath params.EntityPath) modelStats {
	modelDoc, err := s.jem.DB.Model(testContext, modelPath)
	c.Assert(err, gc.IsNil)
	counts := make(map[params.EntityCount]int)
	for name, count := range modelDoc.Counts {
		counts[name] = count.Current
	}
	return modelStats{
		life:             modelDoc.Life,
		unitCount:        modelDoc.Counts[params.UnitCount].Current,
		machineCount:     modelDoc.Counts[params.MachineCount].Current,
		applicationCount: modelDoc.Counts[params.ApplicationCount].Current,
	}
}

func (s *internalSuite) modelLife(c *gc.C, modelPath params.EntityPath) string {
	return s.modelStats(c, modelPath).life
}

func (s *internalSuite) modelUnitCount(c *gc.C, modelPath params.EntityPath) int {
	return s.modelStats(c, modelPath).unitCount
}

func (s *internalSuite) assertLease(c *gc.C, ctlPath params.EntityPath, t time.Time, owner string) {
	ctl, err := s.jem.DB.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, owner)
}

func (s *internalSuite) addJEMController(c *gc.C, ctlPath params.EntityPath) {
	info := s.APIInfo(c)
	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.IsNil)
	err = s.jem.DB.AddController(testContext, &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})
	c.Assert(err, gc.IsNil)
}

func newModel(c *gc.C, st *state.State, name string) *state.State {
	f := factory.NewFactory(st)
	return f.MakeModel(c, &factory.ModelParams{
		Name: name,
	})
}

func newApplication(c *gc.C, st *state.State, name string, numUnits int) {
	f := factory.NewFactory(st)
	svc := f.MakeApplication(c, &factory.ApplicationParams{
		Name: name,
	})
	for i := 0; i < numUnits; i++ {
		f.MakeUnit(c, &factory.UnitParams{
			Application: svc,
		})
	}
}

func addUnitOnMachine(c *gc.C, st *state.State, appId, machineId string) {
	app, err := st.Application(appId)
	c.Assert(err, gc.IsNil)
	machine, err := st.Machine(machineId)
	c.Assert(err, gc.IsNil)
	f := factory.NewFactory(st)
	f.MakeUnit(c, &factory.UnitParams{
		Application: app,
		Machine:     machine,
	})
}

func addFakeController(jshim *jemShimInMemory, path params.EntityPath) {
	jshim.AddController(testContext, &mongodoc.Controller{
		Path:      path,
		UUID:      fmt.Sprintf("some-uuid-%s", path),
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
	})
	jshim.AddModel(testContext, &mongodoc.Model{
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
