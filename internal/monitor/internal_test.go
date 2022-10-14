// Copyright 2016 Canonical Ltd.

package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
	jujuwatcher "github.com/juju/juju/state/watcher"
	jujujujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type internalSuite struct {
	jemtest.BootstrapSuite

	// startTime holds the time that the testing clock is initially
	// set to.
	startTime time.Time

	// clock holds the mock clock used by the monitor package.
	clock *testclock.Clock
}

// We don't want to wait for the usual 5s poll interval.
func init() {
	jujuwatcher.Period = 50 * time.Millisecond
}

var _ = gc.Suite(&internalSuite{})

func (s *internalSuite) SetUpTest(c *gc.C) {
	s.BootstrapSuite.SetUpTest(c)

	// Set up the clock mockery.
	s.clock = testclock.NewClock(epoch)
	s.PatchValue(&Clock, s.clock)
}

var epoch = parseTime("2016-01-01T12:00:00Z")
var testContext = context.Background()

func (s *internalSuite) TestLeaseUpdater(c *gc.C) {
	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(testContext, jemShim{s.JEM}, s.Controller.Path, time.Time{}, "", "jem1")
	c.Assert(err, gc.Equals, nil)

	m := &controllerMonitor{
		ctlPath:     s.Controller.Path,
		leaseExpiry: expiry,
		jem:         jemShim{s.JEM},
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
	ctl := &mongodoc.Controller{Path: s.Controller.Path}
	for a := jujujujutesting.LongAttempt.Start(); a.Next(); {
		err = s.JEM.DB.GetController(testContext, ctl)
		c.Assert(err, gc.Equals, nil)
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
	s.assertLease(c, s.Controller.Path, time.Time{}, "")
}

func (s *internalSuite) TestLeaseUpdaterWhenControllerRemoved(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	// Start the lease updater with no controller existing.
	m := &controllerMonitor{
		ctlPath:     ctlPath,
		leaseExpiry: epoch.Add(leaseExpiryDuration),
		jem:         jemShim{s.JEM},
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
	model1State := newModel(c, s.State, s.StatePool, "model1")
	newApplication(c, model1State, s.StatePool, "model1-app", 2)
	defer model1State.Close()

	model2State := newModel(c, s.State, s.StatePool, "model2")
	newApplication(c, model2State, s.StatePool, "model1-app", 2)
	// Add a co-hosted unit so that we can see a different between units and machines.
	addUnitOnMachine(c, model2State, s.StatePool, "model1-app", "0")
	defer model2State.Close()

	// Add the JEM model entries
	model1Path := params.EntityPath{"bob", "model1"}
	model2Path := params.EntityPath{"bob", "model2"}
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       model1Path,
		Controller: s.Controller.Path,
		UUID:       model1State.ModelUUID(),
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       model2Path,
		Controller: s.Controller.Path,
		UUID:       model2State.ModelUUID(),
	})
	c.Assert(err, gc.Equals, nil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.JEM})
	m := &controllerMonitor{
		ctlPath: s.Controller.Path,
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
			stats:  s.controllerStats(c, s.Controller.Path),
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
			ModelCount:   4,
			ServiceCount: 2,
			UnitCount:    5,
			MachineCount: 4,
		},
		model1: modelStats{
			life:             "alive",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
			unitCount:        2,
			machineCount:     2,
			applicationCount: 1,
		},
		model2: modelStats{
			life:             "alive",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
			unitCount:        3,
			machineCount:     2,
			applicationCount: 1,
		},
	})

	c.Logf("making model2-app2")

	// Add another application and check that the service count and unit counts
	// are maintained.
	newApplication(c, model2State, s.StatePool, "model2-app2", 2)

	jshim.await(c, getAllStats, allStats{
		stats: mongodoc.ControllerStats{
			ModelCount:   4,
			ServiceCount: 3,
			UnitCount:    7,
			MachineCount: 6,
		},
		model1: modelStats{
			life:             "alive",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
			unitCount:        2,
			machineCount:     2,
			applicationCount: 1,
		},
		model2: modelStats{
			life:             "alive",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
			unitCount:        5,
			machineCount:     4,
			applicationCount: 2,
		},
	})

	// Destroy model1 and check that its life status moves to dying, but the rest stays the same.
	model1, err := model1State.Model()
	c.Assert(err, gc.Equals, nil)
	err = model1.Destroy(state.DestroyModelParams{})
	c.Assert(err, gc.Equals, nil)

	jshim.await(c, getAllStats, allStats{
		stats: mongodoc.ControllerStats{
			ModelCount:   4,
			ServiceCount: 3,
			UnitCount:    7,
			MachineCount: 6,
		},
		model1: modelStats{
			life:             "dying",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
			unitCount:        2,
			machineCount:     2,
			applicationCount: 1,
		},
		model2: modelStats{
			life:             "alive",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
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
			ModelCount:   3,
			ServiceCount: 2,
			UnitCount:    5,
			MachineCount: 4,
		},
		model1: modelStats{},
		model2: modelStats{
			life:             "alive",
			status:           "available",
			hasConfig:        true,
			hasStatusSince:   true,
			unitCount:        5,
			machineCount:     4,
			applicationCount: 2,
		},
	})
}

func (s *internalSuite) TestModelRemovedWithFailedWatcher(c *gc.C) {
	// We want to test this scenario:
	// 	- A model is deleted, which calls DestroyModel on the
	//	juju controller but doesn't remove it from jimm.
	//	- Before the watcher update arrives which would tell jimm
	//	to remove the model (perhaps because the
	// 	watcher isn't currently working), the watcher is restarted.
	//	- The model (machines and applications linked to that model too)
	//	should then be removed from jimm, even though it doesn't exist in
	//	the controller.
	//
	// To simulate this, we add a model to the jimm database but don't
	// add it to the underlying Juju state. It should be removed when we
	// find that the model doesn't exist.

	// Add the JEM model entry.
	modelPath := params.EntityPath{"bob", "model-2"}
	modelUUID := "acf6cf9d-c758-45aa-83ad-923731853fdd"
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       modelPath,
		Controller: s.Controller.Path,
		UUID:       modelUUID,
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.UpsertApplication(testContext, &mongodoc.Application{
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: modelUUID,
			Name:      "model-app",
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.UpsertMachine(testContext, &mongodoc.Machine{
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			Id:        "some-machine-id",
			ModelUUID: modelUUID,
		},
	})
	c.Assert(err, gc.Equals, nil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.JEM})
	m := &controllerMonitor{
		ctlPath: s.Controller.Path,
		jem:     jshim,
		ownerId: "jem1",
	}
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
	defer cleanStop(c, m)

	jshim.await(c, func() interface{} {
		stats := s.modelStats(c, modelPath)
		var apps []mongodoc.Application
		err := s.JEM.DB.ForEachApplication(testContext, jimmdb.Eq("info.modeluuid", modelUUID), []string{"_id"}, func(app *mongodoc.Application) error {
			apps = append(apps, *app)
			return nil
		})
		c.Check(err, gc.Equals, nil)
		var machines []mongodoc.Machine
		err = s.JEM.DB.ForEachMachine(testContext, jimmdb.Eq("info.modeluuid", modelUUID), []string{"_id"}, func(m *mongodoc.Machine) error {
			machines = append(machines, *m)
			return nil
		})
		c.Check(err, gc.Equals, nil)
		return modelData{
			models:   stats,
			machines: machines,
			apps:     apps,
		}
	}, modelData{
		models:   modelStats{},
		apps:     []mongodoc.Application{},
		machines: []mongodoc.Machine{},
	})
}

type modelData struct {
	models   modelStats
	apps     []mongodoc.Application
	machines []mongodoc.Machine
}

func (s *internalSuite) TestWatcherUpdatesMachineInfo(c *gc.C) {
	// Add a couple of models and applications with units to watch.
	modelState := newModel(c, s.State, s.StatePool, "model")
	newApplication(c, modelState, s.StatePool, "model-app", 1)
	defer modelState.Close()

	// Add the JEM model entries
	modelPath := params.EntityPath{"bob", "model"}
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       modelPath,
		Controller: s.Controller.Path,
		UUID:       modelState.ModelUUID(),
	})
	c.Assert(err, gc.Equals, nil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.JEM})
	m := &controllerMonitor{
		ctlPath: s.Controller.Path,
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
		life      life.Value
	}

	getMachineInfo := func() interface{} {
		var infos []machineInfo
		err := s.JEM.DB.ForEachMachine(testContext, jimmdb.Eq("info.modeluuid", modelState.ModelUUID()), []string{"_id"}, func(m *mongodoc.Machine) error {
			infos = append(infos, machineInfo{
				modelUUID: m.Info.ModelUUID,
				id:        m.Info.Id,
				life:      m.Info.Life,
			})
			return nil
		})
		c.Assert(err, gc.Equals, nil)
		return infos
	}
	jshim.await(c, getMachineInfo, []machineInfo{{
		modelUUID: modelState.ModelUUID(),
		id:        "0",
		life:      "alive",
	}})
}

func (s *internalSuite) TestWatcherUpdatesApplicationInfo(c *gc.C) {
	// Add a couple of models and applications with units to watch.
	modelState := newModel(c, s.State, s.StatePool, "model")
	newApplication(c, modelState, s.StatePool, "model-app", 1)
	defer modelState.Close()

	// Add the JEM model entries
	modelPath := params.EntityPath{"bob", "model"}
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       modelPath,
		Controller: s.Controller.Path,
		UUID:       modelState.ModelUUID(),
	})
	c.Assert(err, gc.Equals, nil)

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.JEM})
	m := &controllerMonitor{
		ctlPath: s.Controller.Path,
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
	type appInfo struct {
		modelUUID string
		name      string
		life      life.Value
	}

	getApplicationInfo := func() interface{} {
		var infos []appInfo
		err := s.JEM.DB.ForEachApplication(testContext, jimmdb.Eq("info.modeluuid", modelState.ModelUUID()), []string{"_id"}, func(app *mongodoc.Application) error {
			infos = append(infos, appInfo{
				modelUUID: app.Info.ModelUUID,
				name:      app.Info.Name,
				life:      app.Info.Life,
			})
			return nil
		})
		c.Assert(err, gc.Equals, nil)
		return infos
	}
	jshim.await(c, getApplicationInfo, []appInfo{{
		modelUUID: modelState.ModelUUID(),
		name:      "model-app",
		life:      "alive",
	}})
}

func (s *internalSuite) TestWatcherUpdatesApplicationOffer(c *gc.C) {
	modelState, err := s.StatePool.Get(s.Model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()

	f := factory.NewFactory(modelState.State, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.Offer(testContext, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Relation.Name: ep.Relation.Name,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.Model.Path, "test-offer"),
	}
	err = s.JEM.DB.GetApplicationOffer(testContext, &offer1)
	c.Assert(err, jc.ErrorIsNil)

	var updateCount int
	var mu sync.Mutex

	// Start the watcher.
	jshim := newJEMShimWithUpdateNotify(jemShim{s.JEM})
	ushim := newUpdateOfferShim(jshim, func() {
		mu.Lock()
		updateCount++
		mu.Unlock()
	})
	m := &controllerMonitor{
		ctlPath: s.Controller.Path,
		jem:     ushim,
		ownerId: "jem1",
	}
	m.tomb.Go(func() error {
		return m.watcher(testContext)
	})
	defer cleanStop(c, m)

	updateOfferCalled := func() interface{} {
		mu.Lock()
		cnt := updateCount
		mu.Unlock()
		return cnt
	}
	jshim.await(c, updateOfferCalled, 1)
}

func removeModel(c *gc.C, st *state.State) {
	apps, err := st.AllApplications()
	c.Assert(err, gc.Equals, nil)
	for _, app := range apps {
		units, err := app.AllUnits()
		c.Assert(err, gc.Equals, nil)
		for _, unit := range units {
			err := unit.Destroy()
			c.Assert(err, gc.Equals, nil)
			err = unit.EnsureDead()
			c.Assert(err, gc.Equals, nil)
			err = unit.Remove()
			c.Assert(err, gc.Equals, nil)
		}
		err = app.Destroy()
		c.Assert(err, gc.Equals, nil)
	}
	machines, err := st.AllMachines()
	c.Assert(err, gc.Equals, nil)
	for _, machine := range machines {
		err = machine.Destroy()
		c.Assert(err, gc.Equals, nil)
		err = machine.EnsureDead()
		c.Assert(err, gc.Equals, nil)
		err = machine.Remove()
		c.Assert(err, gc.Equals, nil)
	}
	model, err := st.Model()
	c.Assert(err, gc.Equals, nil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, gc.Equals, nil)
	err = st.ProcessDyingModel()
	c.Assert(err, gc.Equals, nil)
	err = st.RemoveDyingModel()
	c.Assert(err, gc.Equals, nil)
}

func (s *internalSuite) TestWatcherKilledWhileDialingAPI(c *gc.C) {
	info := s.APIInfo(c)
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.JEM.DB.InsertController(testContext, &mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    info.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
	})

	c.Assert(err, gc.Equals, nil)

	openCh := make(chan struct{})

	// Start the watcher.
	jshim := jemShimWithAPIOpener{
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			openCh <- struct{}{}
			<-openCh
			return nil, errgo.New("ignored error")
		},
		jemInterface: jemShim{s.JEM},
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
	err := s.JEM.DB.InsertController(testContext, &mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
	})

	c.Assert(err, gc.Equals, nil)

	apiErrorCh := make(chan error)

	// Start the watcher.
	jshim := jemShimWithAPIOpener{
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			return nil, <-apiErrorCh
		},
		jemInterface: jemShim{s.JEM},
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
	ctl := &mongodoc.Controller{Path: ctlPath}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
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
	c.Assert(err, gc.Equals, nil)

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
		ctl, _ := jshim.Controller(testContext, ctlPath)
		return ctl.UnavailableSince
	}
	jshim1.await(c, unavailableSince, time.Time{})
}

// TestControllerMonitor tests that the controllerMonitor can be run with both the
// lease updater and the watcher in place.
func (s *internalSuite) TestControllerMonitor(c *gc.C) {
	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(testContext, jemShim{s.JEM}, s.Controller.Path, time.Time{}, "", "jem1")
	c.Assert(err, gc.Equals, nil)

	jshim := newJEMShimWithUpdateNotify(jemShim{s.JEM})
	m := newControllerMonitor(context.TODO(), controllerMonitorParams{
		ctlPath:     s.Controller.Path,
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
		ctl := &mongodoc.Controller{Path: s.Controller.Path}
		err := s.JEM.DB.GetController(testContext, ctl)
		c.Assert(err, gc.Equals, nil)
		return statsLifeLease{
			Stats:       ctl.Stats,
			ModelLife:   s.modelLife(c, s.Controller.Path),
			LeaseExpiry: ctl.MonitorLeaseExpiry,
			LeaseOwner:  ctl.MonitorLeaseOwner,
		}
	}

	jshim.await(c, getInfo, statsLifeLease{
		Stats: mongodoc.ControllerStats{
			ModelCount: 2,
		},
		ModelLife:   s.modelLife(c, s.Controller.Path),
		LeaseExpiry: epoch.Add(leaseExpiryDuration*5/6 + leaseExpiryDuration),
		LeaseOwner:  "jem1",
	})

	err = worker.Stop(m)
	c.Assert(err, gc.Equals, nil)

	// Check that the lease has been dropped.
	s.assertLease(c, s.Controller.Path, time.Time{}, "")
}

func (s *internalSuite) TestControllerMonitorDiesWithMonitoringStoppedErrorWhenControllerIsRemoved(c *gc.C) {
	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(testContext, jemShim{s.JEM}, s.Controller.Path, time.Time{}, "", "jem1")
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.RemoveController(context.TODO(), &s.Controller)
	c.Assert(err, gc.Equals, nil)
	m := newControllerMonitor(context.TODO(), controllerMonitorParams{
		ctlPath:     s.Controller.Path,
		jem:         jemShim{s.JEM},
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
	c.Assert(err, gc.Equals, nil)

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
	jshim := jemShimWithoutModelSummaryWatcher{jemShim{s.JEM}}

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
	err := s.JEM.DB.GetController(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	if s.Controller.MonitorLeaseExpiry.IsZero() {
		c.Errorf("monitor lease not held")
	}
	if s.Controller.MonitorLeaseOwner != "jem1" && s.Controller.MonitorLeaseOwner != "jem2" {
		c.Errorf("unexpected monitor owner %q", s.Controller.MonitorLeaseOwner)
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
	c.Assert(err, gc.Equals, nil)

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

func (s *internalSuite) TestAllMonitorWithBrokenMongoConnectionWhileCallingStartMonitors(c *gc.C) {
	// There was a bug where we weren't waiting for the controller monitors
	// to send their "done" values if allMonitor.startMonitors returned an error.

	acquireLease := make(chan struct{}, 5)
	jshim0 := newJEMShimInMemory()
	jshim1 := &jemShimWithMonitorLeaseAcquirer{
		acquireMonitorLease: func(ctxt context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
			if ctlPath.Name == "second" {
				// For the second controller, we return an error
				// to trigger the issue.
				return time.Time{}, errgo.New("acquireMonitorLease error")
			}
			// Notify the test that the lease is being acquired.
			// This should only be send on once becasue leaseExpiryDuration
			// is longer than leaseAcquireInterval and we only
			// advance the clock enough to trigger a new lease acquisition.
			acquireLease <- struct{}{}
			return newExpiry, nil
		},
		jemInterface: jshim0,
	}
	openAPI := make(chan struct{})
	defer close(openAPI)
	addFakeController(jshim0, params.EntityPath{"bob", "first"})
	jshim2 := jemShimWithAPIOpener{
		openAPI: func(path params.EntityPath) (jujuAPI, error) {
			// We'll make all the opens block because we
			// don't actually need the controller monitors
			// to do anything.
			<-openAPI
			return nil, errgo.New("no API available")
		},
		jemInterface: jshim1,
	}
	m := newAllMonitor(context.TODO(), jshim2, "jem1")

	// Wait for the first lease to be acquired, then add a new controller
	// before replying so that the next time startMonitors is called,
	// it'll find the new controller.
	select {
	case <-acquireLease:
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for lease acquisition")
	}
	addFakeController(jshim0, params.EntityPath{"bob", "second"})
	// Advance the clock.
	s.clock.Advance(leaseAcquireInterval + 1)
	// The allMonitor should exit because of the lease-acquisition error.
	select {
	case <-m.tomb.Dead():
		c.Assert(m.tomb.Err(), gc.ErrorMatches, `cannot start monitors: cannot acquire lease: acquireMonitorLease error`)
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("timed out waiting for allMonitor to die")
	}
	worker.Stop(m)
}

func (s *internalSuite) controllerStats(c *gc.C, ctlPath params.EntityPath) mongodoc.ControllerStats {
	ctlDoc := mongodoc.Controller{Path: ctlPath}
	err := s.JEM.DB.GetController(testContext, &ctlDoc)
	c.Assert(err, gc.Equals, nil)
	return ctlDoc.Stats
}

// modelStats holds the aspects of a model updated by the monitor.
type modelStats struct {
	life                                                 string
	status                                               string
	message                                              string
	hasConfig                                            bool
	hasStatusSince                                       bool
	unitCount, machineCount, applicationCount, coreCount int
}

func (s *internalSuite) modelStats(c *gc.C, modelPath params.EntityPath) modelStats {
	modelDoc := mongodoc.Model{Path: modelPath}
	err := s.JEM.DB.GetModel(testContext, &modelDoc)
	// The database now deletes any models set to "dead", if the
	// model cannot be found then return that it is "dead".
	if errgo.Cause(err) == params.ErrNotFound {
		return modelStats{}
	}
	c.Assert(err, gc.Equals, nil)
	counts := make(map[params.EntityCount]int)
	for name, count := range modelDoc.Counts {
		counts[name] = count.Current
	}

	ms := modelStats{
		unitCount:        modelDoc.Counts[params.UnitCount].Current,
		machineCount:     modelDoc.Counts[params.MachineCount].Current,
		applicationCount: modelDoc.Counts[params.ApplicationCount].Current,
		coreCount:        modelDoc.Counts[params.CoreCount].Current,
	}
	if modelDoc.Info == nil {
		return ms
	}
	ms.life = modelDoc.Info.Life
	ms.status = modelDoc.Info.Status.Status
	ms.message = modelDoc.Info.Status.Message
	ms.hasConfig = len(modelDoc.Info.Config) > 0
	ms.hasStatusSince = !modelDoc.Info.Status.Since.IsZero()
	return ms
}

func (s *internalSuite) modelLife(c *gc.C, modelPath params.EntityPath) string {
	return s.modelStats(c, modelPath).life
}

func (s *internalSuite) modelUnitCount(c *gc.C, modelPath params.EntityPath) int {
	return s.modelStats(c, modelPath).unitCount
}

func (s *internalSuite) assertLease(c *gc.C, ctlPath params.EntityPath, t time.Time, owner string) {
	ctl := &mongodoc.Controller{Path: ctlPath}
	err := s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, owner)
}

func newModel(c *gc.C, st *state.State, pool *state.StatePool, name string) *state.State {
	f := factory.NewFactory(st, pool)
	return f.MakeModel(c, &factory.ModelParams{
		Name: name,
	})
}

func newApplication(c *gc.C, st *state.State, pool *state.StatePool, name string, numUnits int) {
	f := factory.NewFactory(st, pool)
	svc := f.MakeApplication(c, &factory.ApplicationParams{
		Name: name,
	})
	for i := 0; i < numUnits; i++ {
		f.MakeUnit(c, &factory.UnitParams{
			Application: svc,
		})
	}
}

func addUnitOnMachine(c *gc.C, st *state.State, pool *state.StatePool, appId, machineId string) {
	app, err := st.Application(appId)
	c.Assert(err, gc.Equals, nil)
	machine, err := st.Machine(machineId)
	c.Assert(err, gc.Equals, nil)
	f := factory.NewFactory(st, pool)
	f.MakeUnit(c, &factory.UnitParams{
		Application: app,
		Machine:     machine,
	})
}

func addFakeController(jshim *jemShimInMemory, path params.EntityPath) {
	jshim.AddController(&mongodoc.Controller{
		Path:      path,
		UUID:      fmt.Sprintf("some-uuid-%s", path),
		CACert:    jujujujutesting.CACert,
		AdminUser: "bob",
		HostPorts: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 4567}}},
		Location: map[string]string{
			"cloud":  "test",
			"region": "test1",
		},
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
