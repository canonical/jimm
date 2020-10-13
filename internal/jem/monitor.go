// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
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
	var update bson.D
	if newOwner != "" {
		newExpiry = mongodoc.Time(newExpiry)
		update = bson.D{{"$set", bson.D{
			{"monitorleaseexpiry", newExpiry},
			{"monitorleaseowner", newOwner},
		}}}
	} else {
		newExpiry = time.Time{}
		update = bson.D{{"$unset", bson.D{
			{"monitorleaseexpiry", nil},
			{"monitorleaseowner", nil},
		}}}
	}
	var oldOwnerQuery interface{}
	var oldExpiryQuery interface{}
	if oldOwner == "" {
		oldOwnerQuery = notExistsQuery
	} else {
		oldOwnerQuery = oldOwner
	}
	if oldExpiry.IsZero() {
		oldExpiryQuery = notExistsQuery
	} else {
		oldExpiryQuery = oldExpiry
	}
	err := j.DB.Controllers().Update(bson.D{
		{"path", ctlPath},
		{"monitorleaseexpiry", oldExpiryQuery},
		{"monitorleaseowner", oldOwnerQuery},
	}, update)
	if err == mgo.ErrNotFound {
		// Someone else got there first, or the document has been
		// removed. Technically don't need to distinguish between the
		// two cases, but it's useful to see the different error messages.
		ctl, err := j.DB.Controller(ctx, ctlPath)
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
	err := j.DB.ForEachModel(ctx, bson.D{{"controller", ctlPath}}, nil, func(m *mongodoc.Model) error {
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
