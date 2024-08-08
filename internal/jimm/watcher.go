// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// Publisher defines the interface used by the Watcher
// to publish model summaries.
type Publisher interface {
	Publish(model string, content interface{}) <-chan struct{}
}

// A Watcher watches juju controllers for changes to all models.
type Watcher struct {
	// Database is the database used by the Watcher.
	Database db.Database

	// Dialer is the API dialer JIMM uses to contact juju controllers. if
	// this is not configured all connection attempts will fail.
	Dialer Dialer

	// Pubsub is a pub-sub hub used to publish and subscribe
	// model summaries.
	Pubsub Publisher

	controllerUnavailableChan chan error
	deltaProcessedChan        chan bool
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

// WatchAllModelSummaries starts the watcher which connects to all known
// controllers and monitors them for model summary updates.
// WatchAllModelSummaries polls the database at the given
// interval to find any new controllers to watch. WatchAllModelSummaries blocks
// until either the given context is closed, or there is an error querying
// the database.
func (w *Watcher) WatchAllModelSummaries(ctx context.Context, interval time.Duration) error {
	const op = errors.Op("jimm.WatchAllModelSummaries")

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
				zapctx.Info(ctx, "starting model summary watcher")
				err := w.watchAllModelSummaries(ctx, ctl)
				zapctx.Error(ctx, "model summary watcher stopped", zap.Error(err))
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

func (w *Watcher) dialController(ctx context.Context, ctl *dbmodel.Controller) (api API, err error) {
	const op = errors.Op("jimm.dialController")

	updateController := false
	defer func() {
		if !updateController {
			return
		}
		if uerr := w.Database.UpdateController(ctx, ctl); uerr != nil {
			zapctx.Error(ctx, "cannot set controller available", zap.Error(uerr))
		}
		// Note (alesstimec) This channel is only available in tests.
		if w.controllerUnavailableChan != nil {
			select {
			case w.controllerUnavailableChan <- err:
			default:
			}
		}
	}()

	// connect to the controller
	api, err = w.Dialer.Dial(ctx, ctl, names.ModelTag{}, nil)
	if err != nil {
		ctl.UnavailableSince = db.Now()
		updateController = true

		return nil, errors.E(op, err)
	}
	if ctl.UnavailableSince.Valid {
		ctl.UnavailableSince = sql.NullTime{}
		updateController = true
	}
	return api, nil
}

// A modelState holds the in-memory state of a model for the watcher.
type modelState struct {
	// id is the database id of the model.
	id      uint
	changed bool

	// machines maps the Id of all the machines that have been seen to
	// the number of cores reported.
	machines map[string]int64

	// units stores the ids of all units that have been seen.
	units map[string]bool
}

func (w *Watcher) checkControllerModels(ctx context.Context, ctl *dbmodel.Controller, checks ...func(*dbmodel.Model) error) (map[string]*modelState, error) {
	const op = errors.Op("jimm.checkControllerModels")

	// modelIDs contains the set of models running on the
	// controller that JIMM is interested in.
	modelStates := make(map[string]*modelState)
	// find all the models we expect to get deltas from initially.
	err := w.Database.ForEachControllerModel(ctx, ctl, func(m *dbmodel.Model) error {
		// models without a UUID are currently being initialised
		// and we don't want to check for those yet.
		if !m.UUID.Valid {
			return nil
		}

		for _, check := range checks {
			err := check(m)
			if err != nil {
				return errors.E(op, err)
			}
		}
		modelStates[m.UUID.String] = &modelState{
			id:       m.ID,
			machines: make(map[string]int64),
			units:    make(map[string]bool),
		}
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return modelStates, nil
}

func (w *Watcher) deltaProcessedNotification() {
	if w.deltaProcessedChan != nil {
		select {
		case w.deltaProcessedChan <- true:
		default:
		}
	}
}

// watchController connects to the given controller and watches for model
// changes on the controller.
func (w *Watcher) watchController(ctx context.Context, ctl *dbmodel.Controller) error {
	const op = errors.Op("jimm.watchController")

	// connect to the controller
	api, err := w.dialController(ctx, ctl)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	// start the all watcher
	id, err := api.WatchAllModels(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	defer func() {
		if err := api.AllModelWatcherStop(ctx, id); err != nil {
			zapctx.Error(ctx, "failed to stop all model watcher", zap.Error(err))
		}
	}()

	checkDyingModel := func(m *dbmodel.Model) error {
		if m.Life == state.Dying.String() || m.Life == state.Dead.String() {
			// models that were in the dying state may no
			// longer be on the controller, check if it should
			// be immediately deleted.
			mi := jujuparams.ModelInfo{
				UUID: m.UUID.String,
			}
			if err := api.ModelInfo(ctx, &mi); err != nil {
				// Some versions of juju return unauthorized for models that cannot be found.
				if errors.ErrorCode(err) == errors.CodeNotFound || errors.ErrorCode(err) == errors.CodeUnauthorized {
					if err := w.Database.DeleteModel(ctx, m); err != nil {
						return errors.E(op, err)
					} else {
						return nil
					}
				} else {
					return errors.E(op, err)
				}
			}
		}
		return nil
	}

	// modelStates contains the set of models running on the
	// controller that JIMM is interested in. The function also
	// check for any dying models and deletes them where necessary.
	modelStates, err := w.checkControllerModels(ctx, ctl, checkDyingModel)
	if err != nil {
		return errors.E(op, err)
	}

	modelStatef := func(uuid string) *modelState {
		state, ok := modelStates[uuid]
		if ok {
			return state
		}
		m := dbmodel.Model{
			UUID: sql.NullString{
				String: uuid,
				Valid:  true,
			},
			ControllerID: ctl.ID,
		}
		err := w.Database.GetModel(ctx, &m)
		switch {
		case err == nil:
			st := modelState{
				id:       m.ID,
				machines: make(map[string]int64),
				units:    make(map[string]bool),
			}
			modelStates[uuid] = &st
		case errors.ErrorCode(err) == errors.CodeNotFound:
			modelStates[uuid] = nil
		default:
			zapctx.Error(ctx, "cannot get model", zap.Error(err))
		}
		return modelStates[uuid]
	}

	for {
		// wait for updates from the all watcher.
		deltas, err := api.AllModelWatcherNext(ctx, id)
		if err != nil {
			return errors.E(op, err)
		}
		servermon.MonitorDeltasReceivedCount.WithLabelValues(ctl.UUID).Add(float64(len(deltas)))
		for _, d := range deltas {
			eid := d.Entity.EntityId()
			ctx := zapctx.WithFields(ctx, zap.String("model-uuid", eid.ModelUUID), zap.String("kind", eid.Kind), zap.String("id", eid.Id))
			zapctx.Debug(ctx, "processing delta")
			if err := w.handleDelta(ctx, modelStatef, d); err != nil {
				return errors.E(op, err)
			}
		}
		for k, v := range modelStates {
			if v == nil {
				// If we have cached not to process a model
				// remove it so we check again next time.
				delete(modelStates, k)
				continue
			}
			if v.changed {
				v.changed = false
				// Update changed model.
				err := w.Database.Transaction(func(tx *db.Database) error {
					m := dbmodel.Model{
						ID: v.id,
					}
					if err := tx.GetModel(ctx, &m); err != nil {
						return err
					}
					var machines, cores int64
					for _, n := range v.machines {
						machines++
						cores += n
					}
					m.Cores = cores
					m.Machines = machines
					m.Units = int64(len(v.units))
					if err := tx.UpdateModel(ctx, &m); err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					zapctx.Error(ctx, "cannot get model for update", zap.Error(err))
					continue
				}
			}
		}
	}
}

// watchAllModelSummaries connects to the given controller and watches the
// summary updates.
func (w *Watcher) watchAllModelSummaries(ctx context.Context, ctl *dbmodel.Controller) error {
	const op = errors.Op("jimm.watchAllModelSummaries")

	// connect to the controller
	api, err := w.dialController(ctx, ctl)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	if !api.SupportsModelSummaryWatcher() {
		return errors.E(op, errors.CodeNotSupported)
	}

	// start the model summary watcher
	id, err := api.WatchAllModelSummaries(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	defer func() {
		if err := api.ModelSummaryWatcherStop(ctx, id); err != nil {
			zapctx.Error(ctx, "failed to stop model summary watcher", zap.Error(err))
		}
	}()

	// modelIDs contains the set of models running on the
	// controller that JIMM is interested in.
	modelStates, err := w.checkControllerModels(ctx, ctl)
	if err != nil {
		return errors.E(op, err)
	}

	modelIDf := func(uuid string) uint {
		state, ok := modelStates[uuid]
		if ok {
			return state.id
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
			modelStates[uuid] = &modelState{
				id: m.ID,
			}
			return m.ID
		}
		zapctx.Error(ctx, "cannot get model", zap.Error(err))
		return 0
	}

	for {
		select {
		case <-ctx.Done():
			return errors.E(op, ctx.Err(), "context cancelled")
		default:
		}
		// wait for updates from the all model summary watcher.
		modelSummaries, err := api.ModelSummaryWatcherNext(ctx, id)
		if err != nil {
			return errors.E(op, err)
		}
		// Sanitize the model abstracts.
		for _, summary := range modelSummaries {
			modelID := modelIDf(summary.UUID)
			if modelID == 0 {
				// skip unknown models
				continue
			}
			summary := summary
			admins := make([]string, 0, len(summary.Admins))
			for _, admin := range summary.Admins {
				if names.NewUserTag(admin).IsLocal() {
					// skip any admins that aren't valid external users.
					continue
				}
				admins = append(admins, admin)
			}
			summary.Admins = admins
			w.Pubsub.Publish(summary.UUID, summary)
		}
	}
}

func (w *Watcher) handleDelta(ctx context.Context, modelIDf func(string) *modelState, d jujuparams.Delta) error {
	defer w.deltaProcessedNotification()
	eid := d.Entity.EntityId()
	state := modelIDf(eid.ModelUUID)
	if state == nil {
		return nil
	}
	switch eid.Kind {
	case "application":
		if d.Removed {
			return nil
		}
		return w.updateApplication(ctx, state.id, d.Entity.(*jujuparams.ApplicationInfo))
	case "machine":
		if d.Removed {
			state.changed = true
			delete(state.machines, eid.Id)
			return nil
		}
		var cores int64
		machine := d.Entity.(*jujuparams.MachineInfo)
		if machine.HardwareCharacteristics != nil && machine.HardwareCharacteristics.CpuCores != nil {
			cores = int64(*machine.HardwareCharacteristics.CpuCores)
		}
		sCores, ok := state.machines[eid.Id]
		if !ok || sCores != cores {
			state.machines[eid.Id] = cores
			state.changed = true
		}
	case "model":
		model := dbmodel.Model{
			ID: state.id,
		}
		if d.Removed {
			return w.deleteModel(ctx, &model)
		}
		return w.updateModel(ctx, &model, d.Entity.(*jujuparams.ModelUpdate))
	case "unit":
		if d.Removed {
			state.changed = true
			delete(state.units, eid.Id)
			return nil
		}
		if !state.units[eid.Id] {
			state.changed = true
			state.units[eid.Id] = true
		}
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
		if !(model.Life == state.Dying.String() || model.Life == state.Dead.String()) {
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

func (w *Watcher) updateApplication(ctx context.Context, modelID uint, info *jujuparams.ApplicationInfo) error {
	err := w.Database.Transaction(func(tx *db.Database) error {
		m := dbmodel.Model{
			ID: modelID,
		}
		if err := tx.GetModel(ctx, &m); err != nil {
			return err
		}
		for _, o := range m.Offers {
			if o.ApplicationName != info.Name {
				continue
			}
			if o.CharmURL == info.CharmURL {
				continue
			}
			o.CharmURL = info.CharmURL
			if err := tx.UpdateApplicationOffer(ctx, &o); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		zapctx.Error(ctx, "error updating application", zap.Error(err))
	}
	return nil
}
