// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/version"
)

// ErrLeaseUnavailable is the error cause returned by AcquireMonitorLease
// when it cannot acquire the lease because it is unavailable.
var ErrLeaseUnavailable params.ErrorCode = "cannot acquire lease"

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
// an error with a ErrLeaseUnavailable cause will be returned.
func (j *JEM) AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	update := new(jimmdb.Update)
	if newOwner != "" {
		newExpiry = mongodoc.Time(newExpiry)
		update.Set("monitorleaseexpiry", newExpiry).Set("monitorleaseowner", newOwner)
	} else {
		newExpiry = time.Time{}
		update.Unset("monitorleaseexpiry").Unset("monitorleaseowner")
	}
	var oldOwnerQuery jimmdb.Query
	var oldExpiryQuery jimmdb.Query
	if oldOwner == "" {
		oldOwnerQuery = jimmdb.NotExists("monitorleaseowner")
	} else {
		oldOwnerQuery = jimmdb.Eq("monitorleaseowner", oldOwner)
	}
	if oldExpiry.IsZero() {
		oldExpiryQuery = jimmdb.NotExists("monitorleaseexpiry")
	} else {
		oldExpiryQuery = jimmdb.Eq("monitorleaseexpiry", oldExpiry)
	}
	q := jimmdb.And(jimmdb.Eq("path", ctlPath), oldOwnerQuery, oldExpiryQuery)
	err := j.DB.UpdateControllerQuery(ctx, q, nil, update, false)
	if errgo.Cause(err) == params.ErrNotFound {
		// Someone else got there first, or the document has been
		// removed. Technically don't need to distinguish between the
		// two cases, but it's useful to see the different error messages.
		ctl := &mongodoc.Controller{Path: ctlPath}
		err := j.DB.GetController(ctx, ctl)
		if errgo.Cause(err) == params.ErrNotFound {
			return time.Time{}, errgo.WithCausef(nil, params.ErrNotFound, "controller removed")
		}
		if err != nil {
			return time.Time{}, errgo.Mask(err)
		}
		return time.Time{}, errgo.WithCausef(nil, ErrLeaseUnavailable, "controller has lease taken out by %q expiring at %v", ctl.MonitorLeaseOwner, ctl.MonitorLeaseExpiry.UTC())
	}
	if err != nil {
		return time.Time{}, errgo.Notef(err, "cannot acquire lease")
	}
	return newExpiry, nil
}

// SetModelInfo sets the Info field of then model controlled by the given
// controller that has the given UUID. It does not return an error if
// there is no such model.
func (j *JEM) SetModelInfo(ctx context.Context, ctlPath params.EntityPath, uuid string, info *mongodoc.ModelInfo) error {
	m := mongodoc.Model{
		Controller: ctlPath,
		UUID:       uuid,
	}
	u := new(jimmdb.Update).Set("info", info)
	if err := j.DB.UpdateModel(ctx, &m, u, false); err != nil {
		if errgo.Cause(err) != params.ErrNotFound {
			return errgo.Notef(err, "cannot update model")
		}
	}
	return nil
}

// SetModelLife sets the Info.Life field of the model controlled by the
// given controller that has the given UUID. It does not return an error
// if there is no such model.
func (j *JEM) SetModelLife(ctx context.Context, ctlPath params.EntityPath, uuid string, life string) error {
	m := mongodoc.Model{
		Controller: ctlPath,
		UUID:       uuid,
	}
	u := new(jimmdb.Update).Set("info.life", life)
	if err := j.DB.UpdateModel(ctx, &m, u, false); err != nil {
		if errgo.Cause(err) != params.ErrNotFound {
			return errgo.Notef(err, "cannot update model")
		}
	}
	return nil
}

// ModelUUIDsForController returns the model UUIDs of all the models in the given
// controller.
func (j *JEM) ModelUUIDsForController(ctx context.Context, ctlPath params.EntityPath) ([]string, error) {
	var uuids []string
	err := j.DB.ForEachModel(ctx, jimmdb.Eq("controller", ctlPath), nil, func(m *mongodoc.Model) error {
		uuids = append(uuids, m.UUID)
		return nil
	})
	return uuids, errgo.Mask(err)
}

// UpdateModelCounts updates the count statistics associated with the
// model with the given UUID recording them at the given current time.
// Each counts map entry holds the current count for its key. Counts not
// mentioned in the counts argument will not be affected.
func (j *JEM) UpdateModelCounts(ctx context.Context, ctlPath params.EntityPath, uuid string, counts map[params.EntityCount]int, now time.Time) error {
	// This looks racy but it's actually not too bad. Assuming that
	// two concurrent updates are actually looking at the same
	// controller and hence are setting valid information, they will
	// both be working from a valid set of count values (we
	// only update them all at the same time), so each one will
	// update them to a new valid set. They might each ignore
	// the other's updates but because they're working from the
	// same state information, they should converge correctly.
	m := mongodoc.Model{
		Controller: ctlPath,
		UUID:       uuid,
	}
	if err := j.DB.GetModel(ctx, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	u := new(jimmdb.Update)
	for k, v := range counts {
		c := m.Counts[k]
		UpdateCount(&c, v, now)
		u.Set(string("counts."+k), c)
	}
	return errgo.Mask(j.DB.UpdateModel(ctx, &m, u, false))
}

// SetControllerUnavailableAt marks the controller as having been unavailable
// since at least the given time. If the controller was already marked
// as unavailable, its time isn't changed.
// This method does not return an error when the controller doesn't exist.
func (j *JEM) SetControllerUnavailableAt(ctx context.Context, ctlPath params.EntityPath, t time.Time) error {
	q := jimmdb.And(jimmdb.Eq("path", ctlPath), jimmdb.NotExists("unavailablesince"))
	u := new(jimmdb.Update).Set("unavailablesince", t)
	err := j.DB.UpdateControllerQuery(ctx, q, nil, u, false)
	if err == nil || errgo.Cause(err) == params.ErrNotFound {
		// We don't know whether a not-found error is because there
		// are no controllers with the given name (in which case we want
		// to return a params.ErrNotFound error) or because there was
		// one but it is already unavailable.
		// We could fetch the controller to decide whether it's actually there
		// or not, but because in practice we don't care if we're setting
		// controller-unavailable on a non-existent controller, we'll
		// save the round trip.
		return nil
	}
	return errgo.Mask(err)
}

// SetControllerAvailable marks the given controller as available.
// This method does not return an error when the controller doesn't exist.
func (j *JEM) SetControllerAvailable(ctx context.Context, ctlPath params.EntityPath) error {
	u := new(jimmdb.Update).Unset("unavailablesince")
	err := j.DB.UpdateController(ctx, &mongodoc.Controller{Path: ctlPath}, u, false)
	if err == nil || errgo.Cause(err) == params.ErrNotFound {
		return nil
	}
	return errgo.Mask(err)
}

// SetControllerVersion sets the agent version of the given controller.
// This method does not return an error when the controller doesn't exist.
func (j *JEM) SetControllerVersion(ctx context.Context, ctlPath params.EntityPath, v version.Number) error {
	u := new(jimmdb.Update).Set("version", v)
	err := j.DB.UpdateController(ctx, &mongodoc.Controller{Path: ctlPath}, u, false)
	if err == nil || errgo.Cause(err) == params.ErrNotFound {
		// For symmetry with SetControllerUnavailableAt.
		return nil
	}
	return errgo.Mask(err)
}

// SetControllerStats sets the stats associated with the controller
// with the given path. It returns an error with a params.ErrNotFound
// cause if the controller does not exist.
func (j *JEM) SetControllerStats(ctx context.Context, ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	u := new(jimmdb.Update).Set("stats", stats)
	if err := j.DB.UpdateController(ctx, &mongodoc.Controller{Path: ctlPath}, u, false); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return nil
}

// UpdateMachineInfo updates the information associated with a machine.
func (j *JEM) UpdateMachineInfo(ctx context.Context, ctlPath params.EntityPath, info *jujuparams.MachineInfo) error {
	cloud, region, err := j.modelRegion(ctx, ctlPath, info.ModelUUID)
	if errgo.Cause(err) == params.ErrNotFound {
		// If the model isn't found then it is not controlled by
		// JIMM and we aren't interested in it.
		return nil
	}
	if err != nil {
		return errgo.Notef(err, "cannot find region for model %s:%s", ctlPath, info.ModelUUID)
	}
	machine := mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      cloud,
		Region:     region,
		Info:       info,
	}
	if info.Life == life.Dead {
		err := j.DB.RemoveMachine(ctx, &machine)
		if errgo.Cause(err) == params.ErrNotFound {
			return nil
		}
		return errgo.Mask(err)
	}
	return errgo.Mask(j.DB.UpsertMachine(ctx, &machine))
}

// RemoveControllerMachines removes all the machines attached to the
// given controller.
func (j *JEM) RemoveControllerMachines(ctx context.Context, ctlPath params.EntityPath) error {
	_, err := j.DB.RemoveMachines(ctx, jimmdb.Eq("controller", ctlPath.String()))
	return errgo.Mask(err)
}

// UpdateApplicationInfo updates the information associated with an application.
func (j *JEM) UpdateApplicationInfo(ctx context.Context, ctlPath params.EntityPath, info *jujuparams.ApplicationInfo) error {
	cloud, region, err := j.modelRegion(ctx, ctlPath, info.ModelUUID)
	if errgo.Cause(err) == params.ErrNotFound {
		// If the model isn't found then it is not controlled by
		// JIMM and we aren't interested in it.
		return nil
	}
	if err != nil {
		return errgo.Notef(err, "cannot find region for model %s:%s", ctlPath, info.ModelUUID)
	}
	app := mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      cloud,
		Region:     region,
	}
	if info != nil {
		app.Info = &mongodoc.ApplicationInfo{
			ModelUUID:       info.ModelUUID,
			Name:            info.Name,
			Exposed:         info.Exposed,
			CharmURL:        info.CharmURL,
			OwnerTag:        info.OwnerTag,
			Life:            info.Life,
			Subordinate:     info.Subordinate,
			Status:          info.Status,
			WorkloadVersion: info.WorkloadVersion,
		}
	}
	if info.Life == life.Dead {
		err := j.DB.RemoveApplication(ctx, &app)
		if errgo.Cause(err) == params.ErrNotFound {
			return nil
		}
		return errgo.Mask(err)
	}
	return errgo.Mask(j.DB.UpsertApplication(ctx, &app))
}

// RemoveControllerApplications removes all the applications attached to
// the given controller.
func (j *JEM) RemoveControllerApplications(ctx context.Context, ctlPath params.EntityPath) error {
	_, err := j.DB.RemoveApplications(ctx, jimmdb.Eq("controller", ctlPath.String()))
	return errgo.Mask(err)
}

// modelRegion determines the cloud and region in which a model is contained.
func (j *JEM) modelRegion(ctx context.Context, ctlPath params.EntityPath, uuid string) (params.Cloud, string, error) {
	type cloudRegion struct {
		cloud  params.Cloud
		region string
	}
	key := fmt.Sprintf("%s %s", ctlPath, uuid)
	r, err := j.pool.regionCache.Get(key, func() (interface{}, error) {
		m := mongodoc.Model{
			UUID:       uuid,
			Controller: ctlPath,
		}
		if err := j.DB.GetModel(ctx, &m); err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		return cloudRegion{
			cloud:  m.Cloud,
			region: m.CloudRegion,
		}, nil
	})
	if err != nil {
		return "", "", errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	cr := r.(cloudRegion)
	return cr.cloud, cr.region, nil
}
