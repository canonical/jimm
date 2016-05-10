// Copyright 2016 Canonical Ltd.

package monitor

import (
	"fmt"
	"time"

	"github.com/juju/juju/state/multiwatcher"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

// controllerMonitor is responsible for monitoring a single
// controller.
type controllerMonitor struct {
	// tomb is killed when the controller being monitored
	// has been removed.
	tomb tomb.Tomb

	// leaseExpiry holds the time that the currently held lease
	// will expire. It is maintained by the leaseUpdater goroutine.
	leaseExpiry time.Time

	// jem holds the current JEM database connection.
	jem jemInterface

	// ctlPath holds the path of the controller we're monitoring.
	ctlPath params.EntityPath

	// ownerId holds this agent's name, the owner of the lease.
	ownerId string
}

// controllerMonitorParams holds parameters for creating
// a new controller monitor.
type controllerMonitorParams struct {
	jem         jemInterface
	ctlPath     params.EntityPath
	ownerId     string
	leaseExpiry time.Time
}

// newControllerMonitor starts a new monitor to monitor one controller.
func newControllerMonitor(p controllerMonitorParams) *controllerMonitor {
	m := &controllerMonitor{
		jem:         p.jem,
		ctlPath:     p.ctlPath,
		ownerId:     p.ownerId,
		leaseExpiry: p.leaseExpiry,
	}
	// wrapMonitoringStopped turns errors with an errMonitoringStopped
	// into tomb.ErrDying errors so they'll cause the tomb to exit
	// without an error so the controller monitor will go away without
	// taking down all the other controller monitors too.
	wrapMonitoringStopped := func(f func() error) func() error {
		return func() error {
			err := f()
			if errgo.Cause(err) == errMonitoringStopped {
				return tomb.ErrDying
			}
			return err
		}
	}
	m.tomb.Go(func() error {
		m.tomb.Go(wrapMonitoringStopped(m.leaseUpdater))
		m.tomb.Go(wrapMonitoringStopped(m.watcher))
		m.tomb.Go(func() error {
			<-m.tomb.Dying()
			return nil
		})
		return nil
	})
	return m
}

// Kill implements worker.Worker.Kill by killing the controller monitor.
func (m *controllerMonitor) Kill() {
	m.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait by waiting for
// the controller monitor to terminate.
func (m *controllerMonitor) Wait() error {
	return m.tomb.Wait()
}

// Dead returns a channel which is closed when the controller
// monitor has terminated.
func (m *controllerMonitor) Dead() <-chan struct{} {
	return m.tomb.Dead()
}

var errMonitoringStopped = errgo.New("monitoring stopped because lease lost or controller removed")

// leaseUpdater is responsible for updating the controller's lease
// as long as we still have the lease, the controller still exists,
// and the monitor is still alive.
func (m *controllerMonitor) leaseUpdater() error {
	for {
		// Renew after ¾ of the lease time has passed.
		renewTime := m.leaseExpiry.Add(-leaseExpiryDuration / 4)
		select {
		case <-Clock.After(renewTime.Sub(Clock.Now())):
		case <-m.tomb.Dying():
			// Try to drop the lease because the monitor might
			// not be starting again on this JEM instance.
			if err := m.renewLease(false); err != nil {
				return errgo.NoteMask(err, "cannot drop lease", errgo.Is(errMonitoringStopped))
			}
			return tomb.ErrDying
		}
		// It's time to renew the lease.
		if err := m.renewLease(true); err != nil {
			msg := fmt.Sprintf("cannot renew lease on %v", m.ctlPath)
			return errgo.NoteMask(err, msg, errgo.Is(errMonitoringStopped))
		}
	}
}

// renewLease renews the lease (or drops it if renew is false)
// and updates the m.leaseExpiry to be the new lease expiry time.
//
// If the lease cannot be renewed because someone else
// has acquired it or the controller has been removed,
// it returns an error with an errMonitoringStopped cause.
func (m *controllerMonitor) renewLease(renew bool) error {
	var ownerId string
	if renew {
		ownerId = m.ownerId
	}
	t, err := acquireLease(m.jem, m.ctlPath, m.leaseExpiry, m.ownerId, ownerId)
	if err == nil {
		logger.Debugf("controller %v acquired lease successfully (new time %v)", m.ctlPath, t)
		m.leaseExpiry = t
		return nil
	}
	logger.Infof("controller %v acquire lease failed: %v", m.ctlPath, err)
	return errgo.Mask(err, errgo.Is(errMonitoringStopped))
}

// acquireLease is like jem.JEM.AcquireMonitorLease except that
// it returns errMonitoringStopped if the controller has been
// removed or the lease is unavailable,
// and it always acquires a lease leaseExpiryDuration from now.
func acquireLease(j jemInterface, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner, newOwner string) (time.Time, error) {
	t, err := j.AcquireMonitorLease(ctlPath, oldExpiry, oldOwner, Clock.Now().Add(leaseExpiryDuration), newOwner)
	switch errgo.Cause(err) {
	case nil:
		return t, nil
	case jem.ErrLeaseUnavailable, params.ErrNotFound:
		return time.Time{}, errgo.WithCausef(err, errMonitoringStopped, "%s", "")
	default:
		return time.Time{}, errgo.Mask(err)
	}
}

// watcher runs the controller monitor watcher itself.
// It returns errMonitoringStopped if m.tomb is killed
// or the controller is removed.
func (m *controllerMonitor) watcher() error {
	for {
		logger.Debugf("monitor dialing controller %v", m.ctlPath)
		conn, err := m.dialAPI()
		switch errgo.Cause(err) {
		case nil:
			err := m.watch(conn)
			conn.Close()
			if errgo.Cause(err) == tomb.ErrDying {
				return errMonitoringStopped
			}
			if err != nil {
				logger.Infof("watch controller %v died: %v", m.ctlPath, err)
			}
		case params.ErrNotFound:
			logger.Infof("watch controller %v terminating because controller was removed", m.ctlPath)
			return errMonitoringStopped
		case tomb.ErrDying:
			// The controller has been removed or we've been explicitly stopped.
			return errMonitoringStopped
		case jem.ErrAPIConnection:
			// We've failed to connect to the API. Log the error and
			// try again.
			// TODO update the controller doc with the error?
			logger.Errorf("cannot connect to controller %v: %v", m.ctlPath, err)
			// Sleep for a while so we don't batter the network.
			select {
			case <-m.tomb.Dying():
				// The controllerMonitor is dying.
				return errMonitoringStopped
			case <-Clock.After(apiConnectRetryDuration):
			}
		default:
			// Some other error has happened (probably a mongo
			// failure), so die with an error which will force everything
			// to tear down.
			return errgo.Notef(err, "cannot dial API for controller %v", m.ctlPath)
		}
	}
}

// dialAPI makes an API connection while also monitoring for shutdown.
// If the tomb starts dying while dialing, it returns tomb.ErrDying. If
// we can't make an API connection because the controller has been
// removed, it returns an error with a params.ErrNotFound cause. If it
// can't make a connection because the dial itself failed, it returns an
// error with a jem.ErrAPIConnection cause.
func (m *controllerMonitor) dialAPI() (jujuAPI, error) {
	type apiConnReply struct {
		conn jujuAPI
		err  error
	}
	reply := make(chan apiConnReply, 1)
	// Make an independent copy of the JEM instance
	// because this goroutine might live on beyond
	// the allMonitor's lifetime.
	j := m.jem.Clone()
	go func() {
		// Open the API to the controller's admin model.
		conn, err := j.OpenAPI(m.ctlPath)

		// Close before sending the reply rather than deferring
		// so that if our reply causes everything to be stopped,
		// we know that the JEM is closed before that.
		j.Close()
		reply <- apiConnReply{
			conn: conn,
			err:  err,
		}
	}()
	select {
	case r := <-reply:
		return r.conn, errgo.Mask(r.err, errgo.Is(params.ErrNotFound), errgo.Is(jem.ErrAPIConnection))
	case <-m.tomb.Dying():
		return nil, tomb.ErrDying
	}
}

// watch reads events from the API megawatcher and
// updates runtime stats in the controller document in response
// to those.
func (m *controllerMonitor) watch(conn jujuAPI) error {
	apiw, err := conn.WatchAllModels()
	if err != nil {
		return errgo.Notef(err, "cannot watch all models")
	}
	defer apiw.Stop()

	w := newWatcherState(m.jem, m.ctlPath)
	type reply struct {
		deltas []multiwatcher.Delta
		err    error
	}
	replyc := make(chan reply, 1)
	for {
		go func() {
			// Ideally rpc.Client would have a Go method
			// similar to net/rpc's Go method, so we could
			// avoid making a goroutine each time, but currently
			// it does not.
			d, err := apiw.Next()
			replyc <- reply{d, err}
		}()
		var r reply
		select {
		case r = <-replyc:
		case <-m.tomb.Dying():
			return tomb.ErrDying
		}
		if r.err != nil {
			return errgo.Notef(r.err, "watcher error waiting for next event")
		}
		w.changed = false
		for _, d := range r.deltas {
			if err := w.addDelta(d); err != nil {
				return errgo.Mask(err)
			}
		}
		if w.changed {
			if err := m.jem.SetControllerStats(m.ctlPath, &w.stats); err != nil {
				return errgo.Notef(err, "cannot set controller stats")
			}
		}
	}
}

// watcherState holds the state that's maintained when watching
// a controller.
type watcherState struct {
	jem jemInterface

	// entities holds a map from entity tag to whether it exists.
	entities map[multiwatcher.EntityId]bool

	// ctlPath holds the path to the controller.
	ctlPath params.EntityPath

	// changed holds whether the stats have been updated
	// since the last time it was set to false.
	changed bool

	// stats holds the current known statistics about the controller.
	stats mongodoc.ControllerStats

	// modelLife holds the currently known lifecycle status
	// of all the models in the controller.
	modelLife map[string]multiwatcher.Life
}

func newWatcherState(j jemInterface, ctlPath params.EntityPath) *watcherState {
	return &watcherState{
		jem:       j,
		ctlPath:   ctlPath,
		modelLife: make(map[string]multiwatcher.Life),
		entities:  make(map[multiwatcher.EntityId]bool),
	}
}

func (w *watcherState) addDelta(d multiwatcher.Delta) error {
	logger.Debugf("controller %v saw change %#v", w.ctlPath, d)
	switch e := d.Entity.(type) {
	case *multiwatcher.ModelInfo:
		w.adjustCount(&w.stats.ModelCount, d)
		// TODO update the model information concurrently?
		if d.Removed {
			if err := w.modelRemoved(e.ModelUUID); err != nil {
				return errgo.Notef(err, "cannot mark model as removed")
			}
			break
		}
		if err := w.modelChanged(e); err != nil {
			return errgo.Mask(err)
		}
	case *multiwatcher.UnitInfo:
		w.adjustCount(&w.stats.UnitCount, d)
	case *multiwatcher.ServiceInfo:
		w.adjustCount(&w.stats.ServiceCount, d)
	case *multiwatcher.MachineInfo:
		// TODO for top level machines, increment instance count?
		w.adjustCount(&w.stats.MachineCount, d)
	}
	return nil
}

// adjustCount increments or decrements the value pointed
// to by n depending on whether delta.Removed is true.
// It sets w.changed to true to indicate that something has
// changed and keeps track of whether the entity id exists.
func (w *watcherState) adjustCount(n *int, delta multiwatcher.Delta) {
	id := delta.Entity.EntityId()
	if delta.Removed {
		// Technically there's no need for the test here as we shouldn't
		// get two Removes in a row, but let's be defensive.
		if w.entities[id] {
			w.changed = true
			delete(w.entities, id)
			*n -= 1
		}
		return
	}
	if !w.entities[id] {
		w.entities[id] = true
		w.changed = true
		*n += 1
	}
}

// modelRemoved is called when we know that the model with the
// given UUID has been removed.
func (w *watcherState) modelRemoved(uuid string) error {
	return w.setModelLife(uuid, "dead")
}

// modelChanged is called when we're given new information about
// a model.
func (w *watcherState) modelChanged(m *multiwatcher.ModelInfo) error {
	// TODO set other info about the model here too?
	return w.setModelLife(m.ModelUUID, m.Life)
}

func (w *watcherState) setModelLife(uuid string, life multiwatcher.Life) error {
	if life == w.modelLife[uuid] {
		return nil
	}
	if err := w.jem.SetModelLife(w.ctlPath, uuid, string(life)); err != nil {
		return errgo.Notef(err, "cannot update model")
	}
	w.modelLife[uuid] = life
	return nil
}
