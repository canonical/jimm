// Copyright 2016 Canonical Ltd.

package monitor

import (
	"fmt"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
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
	jem *jem.JEM

	// ctlPath holds the path of the controller we're monitoring.
	ctlPath params.EntityPath

	// ownerId holds this agent's name, the owner of the lease.
	ownerId string
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
func acquireLease(j *jem.JEM, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner, newOwner string) (time.Time, error) {
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
