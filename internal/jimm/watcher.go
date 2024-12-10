// Copyright 2024 Canonical.

package jimm

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// Publisher defines the interface used by the Watcher
// to publish model summaries.
type Publisher interface {
	Publish(model string, content interface{}) <-chan struct{}
}

// A Watcher watches juju controllers for changes to all models.
type Watcher struct {
	// Database is the database used by the Watcher.
	Database *db.Database

	// Dialer is the API dialer JIMM uses to contact juju controllers. if
	// this is not configured all connection attempts will fail.
	Dialer Dialer

	// Pubsub is a pub-sub hub used to publish and subscribe
	// model summaries.
	Pubsub Publisher

	controllerUnavailableChan chan error
	deltaProcessedChan        chan bool
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
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: summary.UUID,
					Valid:  true,
				},
				ControllerID: ctl.ID,
			}
			err := w.Database.GetModel(ctx, &m)
			if err != nil {
				// skip summaries for model not present in JIMM's db
				continue
			}
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
