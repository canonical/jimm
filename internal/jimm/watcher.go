// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// A Watcher watches juju controllers for changes to all models.
type Watcher struct {
	// Database is the database used by the Watcher.
	Database db.Database

	// Dialer is the API dialer JIMM uses to contact juju controllers. if
	// this is not configured all connection attempts will fail.
	Dialer Dialer
}

// Watch starts the watcher which connects to all known controllers and
// monitors them for updates. Watch polls the database at the given
// interval to find any new controllers to watch. Watch blocks until either
// the given context is closed, or there is an error querying the database.
func (w *Watcher) Watch(ctx context.Context, interval time.Duration) error {
	const op = errors.Op("jimm.Watch")

	r := newRunner()
	// Ensure that all started goroutines are completed before we return.
	defer r.wait()

	// Ensure that if the watcher stops because of a database error all
	// the controller connections get closed.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		err := w.Database.ForEachController(ctx, func(ctl *dbmodel.Controller) error {
			ctx := zapctx.WithFields(ctx, zap.String("controller", ctl.Name))
			r.run(ctl.Name, func() {
				zapctx.Info(ctx, "starting controller watcher")
				err := w.watchController(ctx, ctl)
				zapctx.Error(ctx, "controller watcher stopped", zap.Error(err))
			})
			return nil
		})
		if err != nil {
			// Ignore temporary database errors.
			if errors.ErrorCode(err) != errors.CodeDatabaseLocked {
				return errors.E(op, err)
			}
			zapctx.Warn(ctx, "temporary error polling for controllers", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// watchController connects to the given controller and watches for model
// changes on the controller.
func (w *Watcher) watchController(ctx context.Context, ctl *dbmodel.Controller) error {
	const op = errors.Op("jimm.watchController")

	// connect to the controller
	api, err := w.Dialer.Dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		if !ctl.UnavailableSince.Valid {
			ctl.UnavailableSince = db.Now()
			if err := w.Database.UpdateController(ctx, ctl); err != nil {
				zapctx.Error(ctx, "cannot set controller unavailable", zap.Error(err))
			}
		}
		return errors.E(op, err)
	}
	defer api.Close()

	// start the all watcher
	id, err := api.WatchAllModels(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.AllModelWatcherStop(ctx, id)

	// controllerModels contains the set of models running on the
	// controller that JIMM is interested in.
	modelIDs := make(map[string]uint)
	// find all the models we expect to get deltas from initially.
	err = w.Database.ForEachControllerModel(ctx, ctl, func(m *dbmodel.Model) error {
		// models without a UUID are currently being initialised
		// and we don't want to check for those yet.
		if m.UUID.Valid == false {
			return nil
		}

		if m.Life == "dying" {
			// models that were in the dying state may no
			// longer be on the controller, check if it should
			// be immediately deleted.
			mi := jujuparams.ModelInfo{
				UUID: m.UUID.String,
			}
			if err := api.ModelInfo(ctx, &mi); err != nil {
				if errors.ErrorCode(err) == errors.CodeNotFound {
					if err := w.Database.DeleteModel(ctx, m); err != nil {
						return errors.E(op, err)
					} else {
						return nil
					}
				} else {
					return errors.E(op, err)
				}
			}
			// if we found the model then carry on the watcher
			// will report when it is fully removed.
		}
		modelIDs[m.UUID.String] = m.ID
		return nil
	})

	modelIDf := func(uuid string) uint {
		modelID, ok := modelIDs[uuid]
		if ok {
			return modelID
		}
		m := dbmodel.Model{
			UUID: sql.NullString{
				String: uuid,
				Valid:  true,
			},
			ControllerID: ctl.ID,
		}
		err := w.Database.GetModel(ctx, &m)
		if err == nil || errors.ErrorCode(err) == errors.CodeNotFound {
			modelIDs[uuid] = m.ID
			return m.ID
		}
		zapctx.Error(ctx, "cannot get model", zap.Error(err))
		return 0
	}

	for {
		// wait for updates from the all watcher.
		deltas, err := api.AllModelWatcherNext(ctx, id)
		if err != nil {
			return errors.E(op, err)
		}
		for _, d := range deltas {
			if err := w.handleDelta(ctx, modelIDf, d); err != nil {
				return errors.E(op, err)
			}
		}
		for k, v := range modelIDs {
			if v == 0 {
				// If we have cached not to process a model
				// remove it so we check again next time.
				delete(modelIDs, k)
			}
		}
	}
}

func (w *Watcher) handleDelta(ctx context.Context, modelIDf func(string) uint, d jujuparams.Delta) error {
	const op = errors.Op("watcher.handleDelta")

	eid := d.Entity.EntityId()
	modelID := modelIDf(eid.ModelUUID)
	if modelID == 0 {
		return nil
	}
	switch eid.Kind {
	case "application":
		app := dbmodel.Application{
			ModelID: modelID,
			Name:    eid.Id,
		}
		if d.Removed {
			return w.Database.DeleteApplication(ctx, &app)
		}
		return w.updateApplication(ctx, &app, d.Entity.(*jujuparams.ApplicationInfo))
	case "machine":
		machine := dbmodel.Machine{
			ModelID:   modelID,
			MachineID: eid.Id,
		}
		if d.Removed {
			return w.Database.DeleteMachine(ctx, &machine)
		}
		return w.updateMachine(ctx, &machine, d.Entity.(*jujuparams.MachineInfo))
	case "model":
		model := dbmodel.Model{
			ID: modelID,
		}
		if d.Removed {
			return w.deleteModel(ctx, &model)
		}
		return w.updateModel(ctx, &model, d.Entity.(*jujuparams.ModelUpdate))
	case "unit":
		unit := dbmodel.Unit{
			ModelID: modelID,
			Name:    eid.Id,
		}
		if d.Removed {
			return w.Database.DeleteUnit(ctx, &unit)
		}
		return w.updateUnit(ctx, &unit, d.Entity.(*jujuparams.UnitInfo))
	}
	return nil
}

func (w *Watcher) updateApplication(ctx context.Context, app *dbmodel.Application, info *jujuparams.ApplicationInfo) error {
	const op = errors.Op("watcher.updateApplication")

	err := w.Database.Transaction(func(db *db.Database) error {
		if err := db.GetApplication(ctx, app); err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				return err
			}
		}
		app.FromJujuApplicationInfo(*info)
		return db.UpdateApplication(ctx, app)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (w *Watcher) updateMachine(ctx context.Context, machine *dbmodel.Machine, info *jujuparams.MachineInfo) error {
	const op = errors.Op("watcher.updateMachine")

	err := w.Database.Transaction(func(db *db.Database) error {
		if err := db.GetMachine(ctx, machine); err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				return err
			}
		}
		machine.FromJujuMachineInfo(*info)
		return db.UpdateMachine(ctx, machine)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (w *Watcher) deleteModel(ctx context.Context, model *dbmodel.Model) error {
	const op = errors.Op("watcher.deleteModel")

	err := w.Database.Transaction(func(db *db.Database) error {
		if err := db.GetModel(ctx, model); err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				return err
			}
		}
		if model.Life != "dying" {
			// If the model hasn't been marked as dying, don't remove it.
			return nil
		}
		return db.DeleteModel(ctx, model)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (w *Watcher) updateModel(ctx context.Context, model *dbmodel.Model, info *jujuparams.ModelUpdate) error {
	const op = errors.Op("watcher.updateModel")

	err := w.Database.Transaction(func(db *db.Database) error {
		if err := db.GetModel(ctx, model); err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				return err
			}
		}
		model.FromJujuModelUpdate(*info)
		return db.UpdateModel(ctx, model)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (w *Watcher) updateUnit(ctx context.Context, unit *dbmodel.Unit, info *jujuparams.UnitInfo) error {
	const op = errors.Op("watcher.updateUnit")

	err := w.Database.Transaction(func(db *db.Database) error {
		if err := db.GetUnit(ctx, unit); err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				return err
			}
		}
		unit.FromJujuUnitInfo(*info)
		return db.UpdateUnit(ctx, unit)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
