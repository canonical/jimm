// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"sort"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

var (
	// ModelSummaryWatcherNotSupportedError is returned by WatchAllModelSummaries if
	// the controller does not support this functionality
	ModelSummaryWatcherNotSupportedError = errors.E("model summary watcher not supported by the controller", errors.CodeNotSupported)
)

// WatchAllModelSummaries starts watching the summary updates from
// the controller.
func (j *JIMM) WatchAllModelSummaries(ctx context.Context, controller *dbmodel.Controller) (_ func() error, err error) {
	const op = errors.Op("jimm.WatchAllModelSummaries")

	conn, err := j.dial(ctx, controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()

	if !conn.SupportsModelSummaryWatcher() {
		return nil, ModelSummaryWatcherNotSupportedError
	}
	id, err := conn.WatchAllModelSummaries(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}
	watcher := &modelSummaryWatcher{
		conn:    conn,
		id:      id,
		pubsub:  j.Pubsub,
		cleanup: conn.Close,
	}
	go watcher.loop(ctx)
	return watcher.stop, nil
}

type modelSummaryWatcher struct {
	conn    API
	id      string
	pubsub  *pubsub.Hub
	cleanup func() error
}

func (w *modelSummaryWatcher) next(ctx context.Context) ([]jujuparams.ModelAbstract, error) {
	models, err := w.conn.ModelSummaryWatcherNext(ctx, w.id)
	if err != nil {
		return nil, errors.E(err)
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].UUID < models[j].UUID
	})
	// Sanitize the model abstracts.
	for i, m := range models {
		admins := make([]string, 0, len(m.Admins))
		for _, admin := range m.Admins {
			// skip any admins that aren't valid external users.
			if names.NewUserTag(admin).IsLocal() {
				continue
			}
			admins = append(admins, admin)
		}
		models[i].Admins = admins
	}
	return models, nil
}

func (w *modelSummaryWatcher) loop(ctx context.Context) {
	defer func() {
		if err := w.cleanup(); err != nil {
			zapctx.Error(ctx, "cleanup failed", zaputil.Error(err))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		modelSummaries, err := w.next(ctx)
		if err != nil {
			zapctx.Error(ctx, "failed to get next model summary", zaputil.Error(err))
			return
		}
		for _, modelSummary := range modelSummaries {
			w.pubsub.Publish(modelSummary.UUID, modelSummary)
		}
	}
}

func (w *modelSummaryWatcher) stop() error {
	err := w.conn.ModelSummaryWatcherStop(context.TODO(), w.id)
	if err != nil {
		return errors.E(err)
	}
	return nil
}
