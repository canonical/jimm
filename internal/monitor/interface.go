// Copyright 2016 Canonical Ltd.

package monitor

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/version/v2"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
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
	SetControllerStats(ctx context.Context, ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error

	// SetControllerUnavailableAt marks the controller as having been unavailable
	// since at least the given time. If the controller was already marked
	// as unavailable, its time isn't changed.
	// This method does not return an error when the controller doesn't exist.
	SetControllerUnavailableAt(ctx context.Context, ctlPath params.EntityPath, t time.Time) error

	// SetControllerAvailable marks the given controller as available.
	// This method does not return an error when the controller doesn't exist.
	SetControllerAvailable(ctx context.Context, ctlPath params.EntityPath) error

	// SetModelLife sets the Life field of all models controlled
	// by the given controller that have the given UUID.
	// It does not return an error if there are no such models.
	SetModelInfo(ctx context.Context, ctlPath params.EntityPath, uuid string, info *mongodoc.ModelInfo) error

	// DeleteModelWithUUID deletes any model from the database that has the
	// given controller and UUID. No error is returned if no such model
	// exists.
	DeleteModelWithUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) error

	// UpdateModelCounts updates the count statistics associated with the
	// model with the given UUID recording them at the given current time.
	// Each counts map entry holds the current count for its key. Counts not
	// mentioned in the counts argument will not be affected.
	UpdateModelCounts(ctx context.Context, ctlPath params.EntityPath, uuid string, counts map[params.EntityCount]int, now time.Time) error

	// RemoveControllerMachines removes all machines for a controller.
	RemoveControllerMachines(ctx context.Context, ctlPath params.EntityPath) error

	// RemoveControllerApplications removes all applications for a controller.
	RemoveControllerApplications(ctx context.Context, ctlPath params.EntityPath) error

	// UpdateMachineInfo updates the information associated with a machine.
	UpdateMachineInfo(ctx context.Context, ctlPath params.EntityPath, machine *jujuparams.MachineInfo) error

	// UpdateApplicationInfo updates the information associated with an application.
	UpdateApplicationInfo(ctx context.Context, ctlPath params.EntityPath, machine *jujuparams.ApplicationInfo) error

	// AllControllers returns all the controllers in the system.
	AllControllers(ctx context.Context) ([]*mongodoc.Controller, error)

	// ModelUUIDsForController returns the model UUIDs of all the models in the given
	// controller.
	ModelUUIDsForController(ctx context.Context, ctlPath params.EntityPath) ([]string, error)

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
	AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error)

	// Controller retrieve the controller based on the controller path.
	Controller(ctx context.Context, ctlPath params.EntityPath) (*mongodoc.Controller, error)

	// WatchAllModelSummaries starts watching the summary updates from
	// the controller.
	WatchAllModelSummaries(ctx context.Context, ctlPath params.EntityPath) (func() error, error)

	// UpdateApplicationOffer fetches offer details from the controller and updates the
	// application offer in JIMM DB.
	UpdateApplicationOffer(ctx context.Context, ctlPath params.EntityPath, offerUUID string, removed bool) error
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

	// ModelExists reports whether the model with the given UUID
	// exists on the controller.
	ModelExists(uuid string) (bool, error)

	// ServerVersion holds the version of the API server that we are connected to.
	ServerVersion() (version.Number, bool)
}

// allWatcher represents a watcher of all events on a controller.
type allWatcher interface {
	// Next returns a new set of deltas. It will block until there
	// are deltas to return. The first calls to Next on a given watcher
	// will return the entire state of the system without blocking.
	Next() ([]jujuparams.Delta, error)

	// Stop stops the watcher and causes any outstanding Next calls
	// to return an error.
	Stop() error
}
