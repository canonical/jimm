// Copyright 2020 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
)

func init() {
	facadeInit["ModelSummaryWatcher"] = func(r *controllerRoot) []int {
		nextMethod := rpc.Method(r.ModelSummaryWatcherNext)
		stopMethod := rpc.Method(r.ModelSummaryWatcherStop)

		r.AddMethod("ModelSummaryWatcher", 1, "Next", nextMethod)
		r.AddMethod("ModelSummaryWatcher", 1, "Stop", stopMethod)

		return []int{1}
	}
}

// ModelSummaryWatcherNext implements the Next method on the
// ModelSummaryWatcher facade. It returns the next set of model summaries
// when they are available.
func (r *controllerRoot) ModelSummaryWatcherNext(ctx context.Context, objID string) (jujuparams.SummaryWatcherNextResults, error) {
	const op = errors.Op("jujuapi.ModelSummaryWatcherNext")

	w, err := r.watchers.get(objID)
	if err != nil {
		return jujuparams.SummaryWatcherNextResults{}, errors.E(op, err)
	}
	return w.Next()
}

// ModelSummaryWatcherStop implements the Stop method on the
// ModelSummaryWatcher facade.
func (r *controllerRoot) ModelSummaryWatcherStop(ctx context.Context, objID string) error {
	const op = errors.Op("jujuapi.ModelSummaryWatcherStop")

	w, err := r.watchers.get(objID)
	if err != nil {
		return errors.E(op, err)
	}
	return w.Stop()
}

var (
	defaultModelAccessWatcherPeriod = time.Minute
)

type watcherRegistry struct {
	mu       sync.RWMutex
	watchers map[string]*modelSummaryWatcher
}

func (r *watcherRegistry) register(w *modelSummaryWatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.watchers == nil {
		r.watchers = make(map[string]*modelSummaryWatcher)
	}
	r.watchers[w.id] = w
}

func (r *watcherRegistry) get(id string) (*modelSummaryWatcher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w, ok := r.watchers[id]
	if !ok {
		return nil, errors.E(errors.CodeNotFound)
	}
	return w, nil
}

func newModelSummaryWatcher(ctx context.Context, id string, root *controllerRoot, pubsub *pubsub.Hub) (*modelSummaryWatcher, error) {
	accessWatcher := &modelAccessWatcher{
		ctx:             ctx,
		modelGetterFunc: root.allModels,
		period:          defaultModelAccessWatcherPeriod,
	}
	err := accessWatcher.do()
	if err != nil {
		zapctx.Error(ctx, "failed to list user models", zaputil.Error(err))
	}
	go accessWatcher.loop()

	watcher := &modelSummaryWatcher{
		id:        id,
		ctx:       ctx,
		summaries: make(map[string]jujuparams.ModelAbstract),
	}

	cleanupFunction, err := pubsub.SubscribeMatch(accessWatcher.match, watcher.pubsubHandler)
	if err != nil {
		return nil, err
	}
	watcher.cleanup = cleanupFunction

	return watcher, nil
}

type modelSummaryWatcher struct {
	id      string
	ctx     context.Context
	cleanup func()

	mu        sync.RWMutex
	summaries map[string]jujuparams.ModelAbstract
}

func (w *modelSummaryWatcher) pubsubHandler(model string, summaryI interface{}) {
	summary, ok := summaryI.(jujuparams.ModelAbstract)
	if !ok {
		zapctx.Error(
			w.ctx,
			"received unknown message type",
			zap.String("received", fmt.Sprintf("%T", summaryI)),
			zap.String("expected", fmt.Sprintf("%T", summary)),
		)
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.summaries[model] = summary
}

func (w *modelSummaryWatcher) Next() (jujuparams.SummaryWatcherNextResults, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	summaries := make([]jujuparams.ModelAbstract, len(w.summaries))
	i := 0
	for _, summary := range w.summaries {
		summaries[i] = summary
		i++
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UUID < summaries[j].UUID
	})
	return jujuparams.SummaryWatcherNextResults{
		Models: summaries,
	}, nil
}

func (w *modelSummaryWatcher) Stop() error {
	w.cleanup()
	return nil
}

type modelAccessWatcher struct {
	ctx             context.Context
	modelGetterFunc func(context.Context) (jujuparams.UserModelList, error)
	period          time.Duration

	mu     sync.RWMutex
	models map[string]bool
}

func (w *modelAccessWatcher) match(model string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	access, ok := w.models[model]
	if !ok {
		return false
	}
	return access
}

func (w *modelAccessWatcher) loop() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(w.period):
			err := w.do()
			if err != nil {
				zapctx.Error(w.ctx, "failed to list user models", zaputil.Error(err))
			}
		}
	}
}

func (w *modelAccessWatcher) do() error {
	userModelList, err := w.modelGetterFunc(w.ctx)
	if err != nil {
		return err
	}

	models := make(map[string]bool)
	for _, model := range userModelList.UserModels {
		models[model.Model.UUID] = true
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.models = models

	return nil
}
