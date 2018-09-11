// Copyright 2016 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/juju/version"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
	jujuparams "github.com/juju/juju/apiserver/params"
)

// Database wraps an mgo.DB ands adds a number of methods for
// manipulating the database.
type Database struct {
	// sessionPool holds the session pool. This will be
	// reset if there's an unexpected mongodb error.
	sessionPool *mgosession.Pool
	*mgo.Database
}

// checkError inspects the value pointed to by err and marks the database
// connection as dead if it looks like the error is probably
// due to a database connection issue. There may be false positives, but
// the worst that can happen is that we do the occasional unnecessary
// Session.Copy which shouldn't be a problem.
//
// TODO if mgo supported it, a better approach would be to check whether
// the mgo.Session is permanently dead.
func (db *Database) checkError(ctx context.Context, err *error) {
	if *err == nil {
		return
	}
	_, ok := errgo.Cause(*err).(params.ErrorCode)
	if ok {
		return
	}
	db.sessionPool.Reset()

	servermon.DatabaseFailCount.Inc()
	zapctx.Warn(ctx, "discarding mongo session", zaputil.Error(*err))
}

// newDatabase returns a new Database named dbName using
// a session taken from the given pool. The database session
// should be closed after the database is finished with.
func newDatabase(ctx context.Context, pool *mgosession.Pool, dbName string) *Database {
	return &Database{
		sessionPool: pool,
		Database:    pool.Session(ctx).DB(dbName),
	}
}

func (db *Database) clone() *Database {
	return &Database{
		sessionPool: db.sessionPool,
		Database:    db.Database.With(db.Database.Session.Clone()),
	}
}

func (db *Database) ensureIndexes() error {
	indexes := []struct {
		c *mgo.Collection
		i mgo.Index
	}{{
		db.Controllers(),
		mgo.Index{Key: []string{"uuid"}},
	}, {
		db.Machines(),
		mgo.Index{Key: []string{"info.uuid"}},
	}, {
		db.Applications(),
		mgo.Index{Key: []string{"info.uuid"}},
	}, {
		db.Models(),
		mgo.Index{Key: []string{"uuid"}, Unique: true},
	}, {
		db.Credentials(),
		mgo.Index{Key: []string{"path.entitypath.user", "path.cloud"}},
	}}
	for _, idx := range indexes {
		err := idx.c.EnsureIndex(idx.i)
		if err != nil {
			return errgo.Notef(err, "cannot ensure index with keys %v on collection %s", idx.i, idx.c.Name)
		}
	}
	return nil
}

// AddController adds a new controller to the database. It returns an
// error with a params.ErrAlreadyExists cause if there is already a
// controller with the given name. The Id field in ctl will be set from
// its Path field.
func (db *Database) AddController(ctx context.Context, ctl *mongodoc.Controller, cloudRegions []mongodoc.CloudRegion, isPrimary bool) (err error) {
	defer db.checkError(ctx, &err)
	ctl.Id = ctl.Path.String()
	err = db.Controllers().Insert(ctl)
	if err != nil {
		if mgo.IsDup(err) {
			return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
		}
		return errgo.NoteMask(err, "cannot insert controller")
	}
	if len(cloudRegions) == 0 {
		return nil
	}
	err = db.AddCloudRegionsForController(ctx, cloudRegions, ctl.Path, isPrimary)
	if err != nil {
		return errgo.NoteMask(err, "cannot insert controller cloud regions")
	}
	return nil
}

// DeleteController deletes existing controller and all of its
// associated models from the database. It returns an error if
// either deletion fails. If there is no matching controller then the
// error will have the cause params.ErrNotFound.
//
// Note that this operation is not atomic.
func (db *Database) DeleteController(ctx context.Context, path params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	// TODO (urosj) make this operation atomic.
	// Delete its models first.
	info, err := db.Models().RemoveAll(bson.D{{"controller", path}})
	if err != nil {
		return errgo.Notef(err, "error deleting controller models")
	}
	// Then delete the controller.
	err = db.Controllers().RemoveId(path.String())
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", path)
	}
	if err != nil {
		zapctx.Error(ctx, "could not delete controller after removing models",
			zap.Int("model-count", info.Removed),
			zaputil.Error(err),
		)
		return errgo.Notef(err, "cannot delete controller")
	}
	zapctx.Info(ctx, "deleted controller",
		zap.Stringer("controller", path),
		zap.Int("model-count", info.Removed),
	)
	return nil
}

// ModelUUIDsForController returns the model UUIDs of all the models in the given
// controller.
func (db *Database) ModelUUIDsForController(ctx context.Context, ctlPath params.EntityPath) (uuids []string, err error) {
	defer db.checkError(ctx, &err)
	iter := db.Models().Find(bson.D{{"controller", ctlPath}}).Select(bson.D{{"uuid", 1}}).Iter()
	var m mongodoc.Model
	for iter.Next(&m) {
		uuids = append(uuids, m.UUID)
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Mask(err)
	}
	return uuids, nil
}

// AddModel adds a new model to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already an model with the given name.
// If ignores m.Id and sets it from m.Path.
func (db *Database) AddModel(ctx context.Context, m *mongodoc.Model) (err error) {
	defer db.checkError(ctx, &err)
	m.Id = m.Path.String()
	err = db.Models().Insert(m)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert controller model")
	}
	return nil
}

// UpdateLegacyModel updates the given model by adding the Cloud,
// CloudRegion, Credential and DefaultSeries values from the given model
// document. All other values will be ignored.
func (db *Database) UpdateLegacyModel(ctx context.Context, model *mongodoc.Model) (err error) {
	defer db.checkError(ctx, &err)
	update := make(bson.D, 3, 4)
	update[0] = bson.DocElem{"cloud", model.Cloud}
	update[1] = bson.DocElem{"credential", model.Credential}
	update[2] = bson.DocElem{"defaultseries", model.DefaultSeries}
	if model.CloudRegion != "" {
		update = append(update, bson.DocElem{"cloudregion", model.CloudRegion})
	}
	err = db.Models().UpdateId(model.Path.String(), bson.D{{"$set", update}})
	if err == nil {
		return nil
	}
	if errgo.Cause(err) == mgo.ErrNotFound {
		return errgo.WithCausef(err, params.ErrNotFound, "cannot update %s", model.Path)
	}
	return errgo.Notef(err, "cannot update %s", model.Path.String())
}

// SetModelController updates the given model so that it's associated
// with the given controller. This should only be called when migration
// has been initiated for the model and the new controller has been
// verified to exist.
func (db *Database) SetModelController(ctx context.Context, model params.EntityPath, newController params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Models().UpdateId(model.String(), bson.D{{
		"$set", bson.D{{
			"controller", newController,
		}},
	}})
	if errgo.Cause(err) == mgo.ErrNotFound {
		return errgo.WithCausef(err, params.ErrNotFound, "cannot update %s", model)
	}
	return errgo.Mask(err)
}

// DeleteModel deletes an model from the database. If an
// model is also a controller it will not be deleted and an error
// with a cause of params.ErrForbidden will be returned. If the
// model cannot be found then an error with a cause of
// params.ErrNotFound is returned.
func (db *Database) DeleteModel(ctx context.Context, path params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	// TODO when we monitor model health, prohibit this method
	// and delete the model automatically when it is destroyed.
	// Check if model is also a controller.
	err = db.Models().RemoveId(path.String())
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", path)
	}
	if err != nil {
		return errgo.Notef(err, "could not delete model")
	}
	zapctx.Info(ctx, "deleted model", zap.Stringer("model", path))
	return nil
}

// DeleteModelWithUUID deletes any model from the database that has the
// given controller and UUID. No error is returned if no such model
// exists.
func (db *Database) DeleteModelWithUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) (err error) {
	defer db.checkError(ctx, &err)
	if _, err := db.Models().RemoveAll(bson.D{{"uuid", uuid}, {"controller", ctlPath}}); err != nil {
		return errgo.Notef(err, "cannot remove model")
	}
	return nil
}

// SetControllerDeprecated sets whether the given controller is deprecated.
func (db *Database) SetControllerDeprecated(ctx context.Context, ctlPath params.EntityPath, deprecated bool) (err error) {
	defer db.checkError(ctx, &err)
	if deprecated {
		err = db.Controllers().UpdateId(ctlPath.String(), bson.D{{
			"$set", bson.D{{"deprecated", true}},
		}})
	} else {
		// A controller that's not deprecated is stored with no deprecated
		// field for backward compatibility and consistency.
		err = db.Controllers().UpdateId(ctlPath.String(), bson.D{{
			"$unset", bson.D{{"deprecated", nil}},
		}})
	}
	if errgo.Cause(err) == mgo.ErrNotFound {
		return errgo.WithCausef(err, params.ErrNotFound, "cannot update %s", ctlPath)
	}
	return errgo.Mask(err)
}

// Controller returns information on the controller with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (db *Database) Controller(ctx context.Context, path params.EntityPath) (_ *mongodoc.Controller, err error) {
	defer db.checkError(ctx, &err)
	var ctl mongodoc.Controller
	id := path.String()
	err = db.Controllers().FindId(id).One(&ctl)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get controller %q", id)
	}
	return &ctl, nil
}

// Model returns information on the model with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (db *Database) Model(ctx context.Context, path params.EntityPath) (_ *mongodoc.Model, err error) {
	defer db.checkError(ctx, &err)
	id := path.String()
	var m mongodoc.Model
	err = db.Models().FindId(id).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", id)
	}
	return &m, nil
}

// ModelFromUUID returns the document representing the model with the
// given UUID. It returns an error with a params.ErrNotFound cause if the
// model was not found.
func (db *Database) ModelFromUUID(ctx context.Context, uuid string) (_ *mongodoc.Model, err error) {
	defer db.checkError(ctx, &err)
	var m mongodoc.Model
	err = db.Models().Find(bson.D{{"uuid", uuid}}).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", uuid)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", uuid)
	}
	return &m, nil
}

// modelFromControllerAndUUID returns the document representing the model
// with the given UUID on the given controller. It returns an error with
// a params.ErrNotFound cause if the model was not found.
func (db *Database) modelFromControllerAndUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) (_ *mongodoc.Model, err error) {
	defer db.checkError(ctx, &err)
	var m mongodoc.Model
	err = db.Models().Find(bson.D{{"controller", ctlPath}, {"uuid", uuid}}).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", uuid)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", uuid)
	}
	return &m, nil
}

// controllerLocationQuery returns a mongo query that iterates through
// all the public controllers matching the given location attributes,
// including unavailable controllers only if includeUnavailable is true.
// It returns an error if the location attribute keys aren't valid.
func (db *Database) controllerLocationQuery(cloud params.Cloud, region string, includeUnavailable bool) *mgo.Query {
	q := make(bson.D, 0, 4)
	if cloud != "" {
		q = append(q, bson.DocElem{"location.cloud", cloud})
	}
	if region != "" {
		q = append(q, bson.DocElem{"cloud.regions", bson.D{{"$elemMatch", bson.D{{"name", region}}}}})
	}
	q = append(q, bson.DocElem{"public", true})
	if !includeUnavailable {
		q = append(q, bson.DocElem{"unavailablesince", notExistsQuery})
	}
	return db.Controllers().Find(q)
}

// SetControllerVersion sets the agent version of the given controller.
// This method does not return an error when the controller doesn't exist.
func (db *Database) SetControllerVersion(ctx context.Context, ctlPath params.EntityPath, v version.Number) (err error) {
	defer db.checkError(ctx, &err)
	if err = db.Controllers().UpdateId(ctlPath.String(), bson.D{{
		"$set", bson.D{{"version", v}},
	}}); err != nil {
		if err == mgo.ErrNotFound {
			// For symmetry with SetControllerUnavailableAt.
			return nil
		}
		return errgo.Notef(err, "cannot update %v", ctlPath)
	}
	return nil
}

// SetControllerRegions sets the cloud regions for the given controller.
// This method does not return an error when the controller doesn't exist.
func (db *Database) SetControllerRegions(ctx context.Context, ctlPath params.EntityPath, regions []mongodoc.Region) (err error) {
	defer db.checkError(ctx, &err)
	if err = db.Controllers().UpdateId(ctlPath.String(), bson.D{{
		"$set", bson.D{{"cloud.regions", regions}},
	}}); err != nil {
		if err == mgo.ErrNotFound {
			// For symmetry with SetControllerUnavailableAt.
			return nil
		}
		return errgo.Notef(err, "cannot update %v", ctlPath)
	}
	return nil
}

// SetControllerAvailable marks the given controller as available.
// This method does not return an error when the controller doesn't exist.
func (db *Database) SetControllerAvailable(ctx context.Context, ctlPath params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	if err = db.Controllers().UpdateId(ctlPath.String(), bson.D{{
		"$unset", bson.D{{"unavailablesince", nil}},
	}}); err != nil {
		if err == mgo.ErrNotFound {
			// For symmetry with SetControllerUnavailableAt.
			return nil
		}
		return errgo.Notef(err, "cannot update %v", ctlPath)
	}
	return nil
}

// SetControllerUnavailableAt marks the controller as having been unavailable
// since at least the given time. If the controller was already marked
// as unavailable, its time isn't changed.
// This method does not return an error when the controller doesn't exist.
func (db *Database) SetControllerUnavailableAt(ctx context.Context, ctlPath params.EntityPath, t time.Time) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Controllers().Update(
		bson.D{
			{"_id", ctlPath.String()},
			{"unavailablesince", notExistsQuery},
		},
		bson.D{
			{"$set", bson.D{{"unavailablesince", t}}},
		},
	)
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		// We don't know whether the not-found error is because there
		// are no controllers with the given name (in which case we want
		// to return a params.ErrNotFound error) or because there was
		// one but it is already unavailable.
		// We could fetch the controller to decide whether it's actually there
		// or not, but because in practice we don't care if we're setting
		// controller-unavailable on a non-existent controller, we'll
		// save the round trip.
		return nil
	}
	return errgo.Notef(err, "cannot update controller")
}

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
func (db *Database) AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (_ time.Time, err error) {
	defer db.checkError(ctx, &err)
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
	err = db.Controllers().Update(bson.D{
		{"path", ctlPath},
		{"monitorleaseexpiry", oldExpiryQuery},
		{"monitorleaseowner", oldOwnerQuery},
	}, update)
	if err == mgo.ErrNotFound {
		// Someone else got there first, or the document has been
		// removed. Technically don't need to distinguish between the
		// two cases, but it's useful to see the different error messages.
		ctl, err := db.Controller(ctx, ctlPath)
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

// SetControllerStats sets the stats associated with the controller
// with the given path. It returns an error with a params.ErrNotFound
// cause if the controller does not exist.
func (db *Database) SetControllerStats(ctx context.Context, ctlPath params.EntityPath, stats *mongodoc.ControllerStats) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Controllers().UpdateId(
		ctlPath.String(),
		bson.D{{"$set", bson.D{{"stats", stats}}}},
	)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	return errgo.Mask(err)
}

// SetModelLife sets the Info.Life field of all models controlled by the
// given controller that have the given UUID. It does not return an error
// if there are no such models.
func (db *Database) SetModelLife(ctx context.Context, ctlPath params.EntityPath, uuid string, life string) (err error) {
	defer db.checkError(ctx, &err)
	_, err = db.Models().UpdateAll(
		bson.D{{"uuid", uuid}, {"controller", ctlPath}},
		bson.D{{"$set", bson.D{{"info.life", life}}}},
	)
	if err != nil {
		return errgo.Notef(err, "cannot update model")
	}
	return nil
}

// SetModelInfo sets the Info field of all models controlled by the given
// controller that have the given UUID. It does not return an error if
// there are no such models.
func (db *Database) SetModelInfo(ctx context.Context, ctlPath params.EntityPath, uuid string, info *mongodoc.ModelInfo) (err error) {
	defer db.checkError(ctx, &err)
	_, err = db.Models().UpdateAll(
		bson.D{{"uuid", uuid}, {"controller", ctlPath}},
		bson.D{{"$set", bson.D{{"info", info}}}},
	)
	if err != nil {
		return errgo.Notef(err, "cannot update model")
	}
	return nil
}

// UpdateModelCounts updates the count statistics associated with the
// model with the given UUID recording them at the given current time.
// Each counts map entry holds the current count for its key. Counts not
// mentioned in the counts argument will not be affected.
func (db *Database) UpdateModelCounts(ctx context.Context, ctlPath params.EntityPath, uuid string, counts map[params.EntityCount]int, now time.Time) error {
	if err := db.updateCounts(
		ctx,
		db.Models(),
		bson.D{
			{"controller", ctlPath},
			{"uuid", uuid},
		},
		counts,
		now,
	); err != nil {
		return errgo.NoteMask(err, "cannot update model counts", errgo.Is(params.ErrNotFound))
	}
	return nil
}

// GetModelStatuses retrieves the model status from all models.
func (db *Database) GetModelStatuses(ctx context.Context) (statuses params.ModelStatuses, err error) {
	defer db.checkError(ctx, &err)
	query := make(bson.D, 0)
	var models []mongodoc.Model
	if err = db.Models().Find(query).Sort("-CreationTime").All(&models); err != nil {
		return nil, errgo.Mask(err)
	}
	statuses = make([]params.ModelStatus, 0)
	for _, model := range models {
		status := params.ModelStatus{
			ID:         model.Id,
			UUID:       model.UUID,
			Cloud:      string(model.Cloud),
			Region:     model.CloudRegion,
			Created:    model.CreationTime,
			Controller: model.Controller.String(),
			Status:     "unknown",
		}
		if model.Info != nil {
			status.Status = model.Info.Status.Status
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// RemoveControllerMachines removes all of the machine information for
// the given controller.
func (db *Database) RemoveControllerMachines(ctx context.Context, ctlPath params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	if _, err := db.Machines().RemoveAll(bson.D{{"controller", ctlPath.String()}}); err != nil {
		return errgo.Notef(err, "cannot remove machines for controller %v", ctlPath)
	}
	return nil
}

// RemoveControllerApplications removes all of the application information for
// the given controller.
func (db *Database) RemoveControllerApplications(ctx context.Context, ctlPath params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	if _, err := db.Applications().RemoveAll(bson.D{{"controller", ctlPath.String()}}); err != nil {
		return errgo.Notef(err, "cannot remove applications for controller %v", ctlPath)
	}
	return nil
}

// UpdateMachineInfo updates the information associated with a machine.
func (db *Database) UpdateMachineInfo(ctx context.Context, m *mongodoc.Machine) (err error) {
	defer db.checkError(ctx, &err)
	m.Id = m.Controller + " " + m.Info.ModelUUID + " " + m.Info.Id
	if m.Info.Life == "dead" {
		if err := db.Machines().RemoveId(m.Id); err != nil {
			if errgo.Cause(err) == mgo.ErrNotFound {
				return nil
			}
			return errgo.Notef(err, "cannot remove machine %v in model %v", m.Info.Id, m.Info.ModelUUID)
		}
	} else {
		update := bson.D{{
			"$set", bson.D{
				{"info", m.Info},
				{"controller", m.Controller},
				{"cloud", m.Cloud},
				{"region", m.Region},
			},
		}}
		if _, err := db.Machines().UpsertId(m.Id, update); err != nil {
			return errgo.Notef(err, "cannot update machine %v in model %v", m.Info.Id, m.Info.ModelUUID)
		}
	}
	return nil
}

// MachinesForModel returns information on all the machines in the model with
// the given UUID.
func (db *Database) MachinesForModel(ctx context.Context, modelUUID string) (docs []mongodoc.Machine, err error) {
	defer db.checkError(ctx, &err)
	err = db.Machines().Find(bson.D{{"info.modeluuid", modelUUID}}).Sort("_id").All(&docs)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return docs, nil
}

// UpdateApplicationInfo updates the information associated with an application.
func (db *Database) UpdateApplicationInfo(ctx context.Context, app *mongodoc.Application) (err error) {
	defer db.checkError(ctx, &err)
	app.Id = app.Controller + " " + app.Info.ModelUUID + " " + app.Info.Name
	if app.Info.Life == "dead" {
		if err := db.Applications().RemoveId(app.Id); err != nil {
			if errgo.Cause(err) == mgo.ErrNotFound {
				return nil
			}
			return errgo.Notef(err, "cannot remove application %v in model %v", app.Info.Name, app.Info.ModelUUID)
		}
	} else {
		update := bson.D{{
			"$set", bson.D{
				{"info", app.Info},
				{"controller", app.Controller},
				{"cloud", app.Cloud},
				{"region", app.Region},
			},
		}}
		if _, err := db.Applications().UpsertId(app.Id, update); err != nil {
			return errgo.Notef(err, "cannot update application %v in model %v", app.Info.Name, app.Info.ModelUUID)
		}
	}
	return nil
}

// ApplicationsForModel returns information on all the applications in the model with
// the given UUID.
func (db *Database) ApplicationsForModel(ctx context.Context, modelUUID string) (docs []mongodoc.Application, err error) {
	defer db.checkError(ctx, &err)
	err = db.Applications().Find(bson.D{{"info.modeluuid", modelUUID}}).Sort("_id").All(&docs)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return docs, nil
}

// updateCounts updates the count statistics for an document in the given collection
// which should be uniquely specified  by the query.
// Each counts map entry holds the current count for its key.
// Counts not mentioned in the counts argument will not be affected.
func (db *Database) updateCounts(ctx context.Context, c *mgo.Collection, query bson.D, values map[params.EntityCount]int, now time.Time) (err error) {
	defer db.checkError(ctx, &err)

	// This looks racy but it's actually not too bad. Assuming that
	// two concurrent updates are actually looking at the same
	// controller and hence are setting valid information, they will
	// both be working from a valid set of count values (we
	// only update them all at the same time), so each one will
	// update them to a new valid set. They might each ignore
	// the other's updates but because they're working from the
	// same state information, they should converge correctly.
	var oldCounts struct {
		Counts map[params.EntityCount]params.Count
	}
	err = c.Find(query).Select(bson.D{{"counts", 1}}).One(&oldCounts)
	if err != nil {
		if err == mgo.ErrNotFound {
			return params.ErrNotFound
		}
		return errgo.Mask(err)
	}
	newCounts := make(bson.D, 0, len(values))
	for name, val := range values {
		count := oldCounts.Counts[name]
		UpdateCount(&count, val, now)
		newCounts = append(newCounts, bson.DocElem{string("counts." + name), count})
	}
	err = c.Update(query, bson.D{{"$set", newCounts}})
	if err != nil {
		if err == mgo.ErrNotFound {
			// This can happen if the document has been
			// removed since we retrieved it. The error
			// should be the same in this case (and we want
			// to prevent the mongo session being discarded
			// if it happens).
			return params.ErrNotFound
		}
		return errgo.Notef(err, "cannot update count")
	}
	return nil
}

// updateCredential stores the given credential in the database. If a
// credential with the same name exists it is overwritten.
func (db *Database) updateCredential(ctx context.Context, cred *mongodoc.Credential) (err error) {
	defer db.checkError(ctx, &err)
	update := bson.D{{
		"type", cred.Type,
	}, {
		"label", cred.Label,
	}, {
		"attributes", cred.Attributes,
	}, {
		"revoked", cred.Revoked,
	}}
	if len(cred.ACL.Read) > 0 {
		update = append(update, bson.DocElem{"acl", cred.ACL})
	}
	id := cred.Path.String()
	_, err = db.Credentials().UpsertId(id, bson.D{{
		"$set", update,
	}, {
		"$setOnInsert", bson.D{{
			"path", cred.Path,
		}},
	}})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential gets the specified credential. If the credential cannot be
// found the returned error will have a cause of params.ErrNotFound.
func (db *Database) Credential(ctx context.Context, path params.CredentialPath) (_ *mongodoc.Credential, err error) {
	defer db.checkError(ctx, &err)
	var cred mongodoc.Credential
	err = db.Credentials().FindId(path.String()).One(&cred)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", path)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get credential %q", path)
	}
	return &cred, nil
}

// credentialAddController stores the fact that the credential with the
// given user, cloud and name is present on the given controller.
func (db *Database) credentialAddController(ctx context.Context, credential params.CredentialPath, controller params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Credentials().UpdateId(credential.String(), bson.D{{
		"$addToSet", bson.D{{"controllers", controller}},
	}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", credential)
		}
		return errgo.Notef(err, "cannot update credential %q", credential)
	}
	return nil
}

// credentialRemoveController stores the fact that the credential with
// the given user, cloud and name is not present on the given controller.
func (db *Database) credentialRemoveController(ctx context.Context, credential params.CredentialPath, controller params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Credentials().UpdateId(credential.String(), bson.D{{
		"$pull", bson.D{{"controllers", controller}},
	}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", credential)
		}
		return errgo.Notef(err, "cannot update credential %q", credential)
	}
	return nil
}

// Cloud gets the details of the given cloud.
//
// Note that there may be many controllers with the given cloud name. We
// return an arbitrary choice, assuming that cloud definitions are the
// same across all possible controllers.
func (db *Database) Cloud(ctx context.Context, cloud params.Cloud) (_ *mongodoc.Cloud, err error) {
	defer db.checkError(ctx, &err)
	var cloudRegion mongodoc.CloudRegion
	err = db.CloudRegions().Find(bson.D{{"cloud", cloud}, {"region", ""}}).One(&cloudRegion)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q not found", cloud)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get cloud %q", cloud)
	}

	return &mongodoc.Cloud{
		Name:             cloudRegion.Cloud,
		ProviderType:     cloudRegion.ProviderType,
		AuthTypes:        cloudRegion.AuthTypes,
		Endpoint:         cloudRegion.Endpoint,
		IdentityEndpoint: cloudRegion.IdentityEndpoint,
		StorageEndpoint:  cloudRegion.StorageEndpoint,
	}, nil
}

func (db *Database) Clouds(ctx context.Context) (_ map[string]jujuparams.Cloud, err error) {
	defer db.checkError(ctx, &err)
	iter := db.CloudRegions().Find(bson.D{}).Select(bson.D{}).Iter()
	results := map[string]jujuparams.Cloud{}
	var v mongodoc.CloudRegion
	for iter.Next(&v) {
		key := names.NewCloudTag(string(v.Cloud)).String()
		cr, _ := results[key]
		if v.Region == "" {
			// v is a cloud
			cr.Type = v.ProviderType
			cr.AuthTypes = v.AuthTypes
			cr.Endpoint = v.Endpoint
			cr.IdentityEndpoint = v.IdentityEndpoint
			cr.StorageEndpoint = v.StorageEndpoint
		} else {
			// v is a region
			cr.Regions = append(cr.Regions, jujuparams.CloudRegion{
				Name:             v.Region,
				Endpoint:         v.Endpoint,
				IdentityEndpoint: v.IdentityEndpoint,
				StorageEndpoint:  v.StorageEndpoint,
			})
		}
		results[key] = cr
	}
	return results, nil
}

// AddCloudRegionsForController adds new cloud regions to the database for a given controller.
func (db *Database) AddCloudRegionsForController(ctx context.Context, cloudRegions []mongodoc.CloudRegion, ctl params.EntityPath, isPrimary bool) (err error) {
	defer db.checkError(ctx, &err)
	for _, cr := range cloudRegions {
		cr.Id = fmt.Sprintf("%s/%s", cr.Cloud, cr.Region)
		update := make(bson.D, 2, 2)
		update[0] = bson.DocElem{
			"$setOnInsert", bson.D{
				{"cloud", cr.Cloud},
				{"region", cr.Region},
				{"providertype", cr.ProviderType},
				{"authtypes", cr.AuthTypes},
				{"endpoint", cr.Endpoint},
				{"identityendpoint", cr.IdentityEndpoint},
				{"storageendpoint", cr.StorageEndpoint},
				{"acl", cr.ACL},
			}}
		if isPrimary {
			update[1] = bson.DocElem{"$addToSet", bson.D{{"primarycontrollers", ctl}}}
		} else {
			update[1] = bson.DocElem{"$addToSet", bson.D{{"secondarycontrollers", ctl}}}
		}

		if _, err := db.CloudRegions().UpsertId(cr.Id, update); err != nil {
			return errgo.Notef(err, "cannot add cloud regions")
		}
	}
	return nil
}

//
func (db *Database) CloudRegion(ctx context.Context, name params.Cloud, region string) (_ *mongodoc.CloudRegion, err error) {
	defer db.checkError(ctx, &err)
	var cloudRegion mongodoc.CloudRegion
	err = db.CloudRegions().Find(bson.D{{"cloud", name}, {"region", region}}).One(&cloudRegion)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q region %q not found", name, region)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get cloud %q region %q", name, region)
	}

	return &cloudRegion, nil
}

// setCredentialUpdates marks all the controllers in the given ctlPaths
// as requiring an update to the credential with the given credPath.
func (db *Database) setCredentialUpdates(ctx context.Context, ctlPaths []params.EntityPath, credPath params.CredentialPath) (err error) {
	defer db.checkError(ctx, &err)
	_, err = db.Controllers().UpdateAll(bson.D{{
		"path", bson.D{{
			"$in", ctlPaths,
		}},
	}}, bson.D{{
		"$addToSet", bson.D{{
			"updatecredentials", credPath}},
	}})
	if err != nil {
		return errgo.Mask(err)

	}
	return nil
}

// clearCredentialUpdate removes the record indicating that the given
// controller needs to update the given credential.
func (db *Database) clearCredentialUpdate(ctx context.Context, ctlPath params.EntityPath, credPath params.CredentialPath) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Controllers().UpdateId(
		ctlPath.String(),
		bson.D{{
			"$pull",
			bson.D{{
				"updatecredentials",
				credPath,
			}},
		}},
	)
	if err != nil {
		if errgo.Cause(err) == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", ctlPath)
		}
		return errgo.Mask(err)
	}
	return nil
}

var selectACL = bson.D{{"acl", 1}}

// GetACL retrieves the ACL for the document at path in coll, which must
// have been obtained from db. If the document is not found, the
// returned error will have the cause params.ErrNotFound.
func (db *Database) GetACL(ctx context.Context, coll *mgo.Collection, path params.EntityPath) (_ params.ACL, err error) {
	defer db.checkError(ctx, &err)
	var doc struct {
		ACL params.ACL
	}
	if err = coll.FindId(path.String()).Select(selectACL).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			err = params.ErrNotFound
		}
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return doc.ACL, nil
}

// SetACL sets the ACL for the path document in c (which must
// have been obtained from db) to be equal to acl.
func (db *Database) SetACL(ctx context.Context, c *mgo.Collection, path params.EntityPath, acl params.ACL) (err error) {
	defer db.checkError(ctx, &err)
	err = c.UpdateId(path.String(), bson.D{{"$set", bson.D{{"acl", acl}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Grant updates the ACL for the path document in c (which must
// have been obtained from db) to include user.
func (db *Database) Grant(ctx context.Context, c *mgo.Collection, path params.EntityPath, user params.User) (err error) {
	defer db.checkError(ctx, &err)
	err = c.UpdateId(path.String(), bson.D{{"$addToSet", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Revoke updates the ACL for the path document in c (which must
// have been obtained from db) to not include user.
func (db *Database) Revoke(ctx context.Context, c *mgo.Collection, path params.EntityPath, user params.User) (err error) {
	defer db.checkError(ctx, &err)
	err = c.UpdateId(path.String(), bson.D{{"$pull", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// GrantModel updates the ACL for the document with the given path in the
// model collection. Permission is granted to the given access level and
// all lower levels.
func (db *Database) GrantModel(ctx context.Context, path params.EntityPath, user params.User, access string) (err error) {
	defer db.checkError(ctx, &err)
	aclUpdates := make(bson.D, 0, 3)
	switch access {
	case "admin":
		aclUpdates = append(aclUpdates, bson.DocElem{"acl.admin", user})
		fallthrough
	case "write":
		aclUpdates = append(aclUpdates, bson.DocElem{"acl.write", user})
		fallthrough
	case "read":
		aclUpdates = append(aclUpdates, bson.DocElem{"acl.read", user})
	default:
		return errgo.Newf("%q model access not valid", access)
	}
	err = db.Models().UpdateId(path.String(), bson.D{{"$addToSet", aclUpdates}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// RevokeModel updates the ACL for the document with the given path in
// the model collection. Permission is revoked from the given access
// level and all higher levels.
func (db *Database) RevokeModel(ctx context.Context, path params.EntityPath, user params.User, access string) (err error) {
	defer db.checkError(ctx, &err)
	aclUpdates := make(bson.D, 0, 3)
	switch access {
	case "read":
		aclUpdates = append(aclUpdates, bson.DocElem{"acl.read", user})
		fallthrough
	case "write":
		aclUpdates = append(aclUpdates, bson.DocElem{"acl.write", user})
		fallthrough
	case "admin":
		aclUpdates = append(aclUpdates, bson.DocElem{"acl.admin", user})
	default:
		return errgo.Newf("%q model access not valid", access)
	}
	err = db.Models().UpdateId(path.String(), bson.D{{"$pull", aclUpdates}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// CheckReadACL checks that the entity with the given path in the given
// collection (which must have been obtained from db) can be read by the
// currently authenticated user.
func (db *Database) CheckReadACL(ctx context.Context, c *mgo.Collection, path params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	// The user can always access their own entities.
	if err := auth.CheckIsUser(ctx, path.User); err == nil {
		return nil
	}
	acl, err := db.GetACL(ctx, c, path)
	if errgo.Cause(err) == params.ErrNotFound {
		// The document is not found - and we've already checked
		// that the currently authenticated user cannot speak for
		// path.User, so return an unauthorized error to stop
		// people probing for the existence of other people's entities.
		return params.ErrUnauthorized
	}
	if err := auth.CheckACL(ctx, acl.Read); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

// CanReadIter returns an iterator that iterates over items in the given
// iterator, which must have been derived from db, returning only those
// that the currently logged in user has permission to see.
//
// The API matches that of mgo.Iter.
func (db *Database) NewCanReadIter(ctx context.Context, iter *mgo.Iter) *CanReadIter {
	return &CanReadIter{
		ctx:  ctx,
		iter: iter,
		db:   db,
	}
}

// CanReadIter represents an iterator that returns only items
// that the currently authenticated user has read access to.
type CanReadIter struct {
	ctx  context.Context
	db   *Database
	iter *mgo.Iter
	err  error
	n    int
}

// Next reads the next item from the iterator into the given
// item and returns whether it has done so.
func (iter *CanReadIter) Next(item auth.ACLEntity) bool {
	if iter.err != nil {
		return false
	}
	for iter.iter.Next(item) {
		iter.n++
		if err := auth.CheckCanRead(iter.ctx, item); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				// No permissions to look at the entity, so don't include
				// it in the results.
				continue
			}
			iter.err = errgo.Mask(err)
			iter.iter.Close()
			return false
		}
		return true
	}
	return false
}

func (iter *CanReadIter) Close() error {
	iter.iter.Close()
	return iter.Err()
}

// Err returns any error encountered when iterating.
func (iter *CanReadIter) Err() error {
	if iter.err != nil {
		// If iter.err is set, it's because CheckCanRead
		// has failed, and that doesn't talk to mongo,
		// so no use in calling checkError in that case.
		return iter.err
	}
	err := iter.iter.Err()
	iter.db.checkError(iter.ctx, &err)
	return err
}

// Count returns the total number of items traversed
// by the iterator, including items that were not returned
// because they were unauthorized.
func (iter *CanReadIter) Count() int {
	return iter.n
}

func (db *Database) Collections() []*mgo.Collection {
	return []*mgo.Collection{
		db.Audits(),
		db.Applications(),
		db.CloudRegions(),
		db.Controllers(),
		db.Credentials(),
		db.Macaroons(),
		db.Machines(),
		db.Models(),
	}
}

func (db *Database) Applications() *mgo.Collection {
	return db.C("applications")
}

func (db *Database) Audits() *mgo.Collection {
	return db.C("audits")
}

func (db *Database) CloudRegions() *mgo.Collection {
	return db.C("cloudregions")
}

func (db *Database) Controllers() *mgo.Collection {
	return db.C("controllers")
}

func (db *Database) Credentials() *mgo.Collection {
	return db.C("credentials")
}

func (db *Database) Macaroons() *mgo.Collection {
	return db.C("macaroons")
}

func (db *Database) Machines() *mgo.Collection {
	return db.C("machines")
}

func (db *Database) Models() *mgo.Collection {
	return db.C("models")
}

func (db *Database) C(name string) *mgo.Collection {
	if db.Database == nil {
		panic(fmt.Sprintf("cannot get collection %q because JEM closed", name))
	}
	return db.Database.C(name)
}

// sessionStatus records the current status of a mgo session.
type sessionStatus int32

// setDead marks the session as dead, so that it won't be
// reused for new JEM instances.
func (s *sessionStatus) setDead() {
	atomic.StoreInt32((*int32)(s), 1)
}

// isDead reports whether the session has been marked as dead.
func (s *sessionStatus) isDead() bool {
	return atomic.LoadInt32((*int32)(s)) != 0
}
