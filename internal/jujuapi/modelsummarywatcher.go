// Copyright 2020 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/jujuapi/rpc"
	"github.com/canonical/jimm/internal/pubsub"
	"github.com/canonical/jimm/internal/zapctx"
	"github.com/canonical/jimm/internal/zaputil"
	"github.com/canonical/jimm/params"
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

func (r *controllerRoot) ModelSummaryWatcherNext(ctx context.Context, objID string) (jujuparams.SummaryWatcherNextResults, error) {
	w, err := r.watchers.get(objID)
	if err != nil {
		return jujuparams.SummaryWatcherNextResults{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return w.Next()
}

func (r *controllerRoot) ModelSummaryWatcherStop(ctx context.Context, objID string) error {
	w, err := r.watchers.get(objID)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
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

func (r *watcherRegistry) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, w := range r.watchers {
		err := w.Stop()
		if err != nil {
			zapctx.Error(context.Background(), "failed to stop a model summary watcher", zaputil.Error(err))
		}
	}
	r.watchers = nil
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

	if r.watchers == nil {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}

	w, ok := r.watchers[id]
	if !ok {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	return w, nil
}

func newModelSummaryWatcher(id string, root *controllerRoot, pubsub *pubsub.Hub) (*modelSummaryWatcher, error) {
	ctx, cancelContext := context.WithCancel(context.Background())

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
		cancelContext()
		return nil, errgo.Mask(err)
	}
	watcher.cleanup = func() {
		cancelContext()
		cleanupFunction()
	}

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
	if w.cleanup != nil {
		w.cleanup()
	}
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
		return errgo.Mask(err)
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
