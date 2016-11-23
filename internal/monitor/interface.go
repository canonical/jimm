// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	"github.com/juju/juju/state/multiwatcher"
	"golang.org/x/net/context"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

// jemInterface holds the interface required by allMonitor to
// talk to the JEM database. It is defined as an interface so
// it can be faked for testing purposes.
type jemInterface interface {
	// Close closes the JEM instance. This should be called when
	// the JEM instance is finished with.
	Close()

	// Clone returns an independent copy of the receiver
	// that uses a cloned database connection. The
	// returned value must be closed after use.
	Clone() jemInterface

	// SetControllerStats sets the stats associated with the controller
	// with the given path. It returns an error with a params.ErrNotFound
	// cause if the controller does not exist.
	SetControllerStats(ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error

	// SetControllerUnavailableAt marks the controller as having been unavailable
	// since at least the given time. If the controller was already marked
	// as unavailable, its time isn't changed.
	// This method does not return an error when the controller doesn't exist.
	SetControllerUnavailableAt(ctlPath params.EntityPath, t time.Time) error

	// SetControllerAvailable marks the given controller as available.
	// This method does not return an error when the controller doesn't exist.
	SetControllerAvailable(ctlPath params.EntityPath) error

	// SetModelLife sets the Life field of all models controlled
	// by the given controller that have the given UUID.
	// It does not return an error if there are no such models.
	SetModelLife(ctlPath params.EntityPath, uuid string, life string) error

	// UpdateModelCounts updates the count statistics associated with the
	// model with the given UUID recording them at the given current time.
	// Each counts map entry holds the current count for its key. Counts not
	// mentioned in the counts argument will not be affected.
	UpdateModelCounts(uuid string, counts map[params.EntityCount]int, now time.Time) error

	// UpdateMachineInfo updates the information associated with a machine.
	UpdateMachineInfo(machine *multiwatcher.MachineInfo) error

	// AllControllers returns all the controllers in the system.
	AllControllers() ([]*mongodoc.Controller, error)

	// OpenAPI opens an API connection to the model with the given path
	// and returns it along with the information used to connect.
	// If the model does not exist, the error will have a cause
	// of params.ErrNotFound.
	//
	// If the model API connection could not be made, the error
	// will have a cause of jem.ErrAPIConnection.
	//
	// The returned connection must be closed when finished with.
	OpenAPI(context.Context, params.EntityPath) (jujuAPI, error)

	// AcquireMonitorLease acquires or renews the lease on a controller.
	// The lease will only be changed if the lease in the database
	// has the given old expiry time and owner.
	// When acquired, the lease will have the given new owner
	// and expiration time.
	//
	// If newOwner is empty, the lease will be dropped, the
	// returned time will be zero and newExpiry will be ignored.
	//
	// If the controller has been removed, an error with a params.ErrNotFound
	// cause will be returned. If the lease has been obtained by someone else
	// an error with a jem.ErrLeaseUnavailable cause will be returned.
	AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error)

	// ControllerUpdateCredentials updates the given controller by updating
	// all credentials listed in ctl.UpdateCredentials.
	ControllerUpdateCredentials(ctx context.Context, ctlPath params.EntityPath) error
}

// jujuAPI represents an API connection to a Juju controller.
type jujuAPI interface {
	// Evict closes the connection and removes it from the API connection cache.
	Evict()

	// WatchAllModels returns an allWatcher, from which you can request
	// the Next collection of Deltas (for all models).
	WatchAllModels() (allWatcher, error)

	// Close closes the API connection.
	Close() error
}

// allWatcher represents a watcher of all events on a controller.
type allWatcher interface {
	// Next returns a new set of deltas. It will block until there
	// are deltas to return. The first calls to Next on a given watcher
	// will return the entire state of the system without blocking.
	Next() ([]multiwatcher.Delta, error)

	// Stop stops the watcher and causes any outstanding Next calls
	// to return an error.
	Stop() error
}
