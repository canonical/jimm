// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
)

func NewModelSummaryWatcher() *modelSummaryWatcher {
	return &modelSummaryWatcher{
		summaries: make(map[string]jujuparams.ModelAbstract),
	}
}

func PublishToWatcher(w *modelSummaryWatcher, model string, data interface{}) {
	w.pubsubHandler(model, data)
}

func NewModelAccessWatcher(ctx context.Context, period time.Duration, modelGetterFunc func(context.Context) (jujuparams.UserModelList, error)) *modelAccessWatcher {
	return &modelAccessWatcher{
		ctx:             ctx,
		modelGetterFunc: modelGetterFunc,
		period:          period,
		models:          make(map[string]bool),
	}
}

func ModelAccessWatcherMatch(w *modelAccessWatcher, model string) bool {
	return w.match(model)
}

func RunModelAccessWatcher(w *modelAccessWatcher) {
	go w.loop()
}
