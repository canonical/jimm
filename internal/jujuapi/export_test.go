// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	jujuparams "github.com/juju/juju/rpc/params"
)

var (
	NewModelAccessWatcher = newModelAccessWatcher
	JujuTagFromTuple      = jujuTagFromTuple
	ParseTag              = parseTag
	ResolveTupleObject    = resolveTupleObject
)

func NewModelSummaryWatcher() *modelSummaryWatcher {
	return &modelSummaryWatcher{
		summaries: make(map[string]jujuparams.ModelAbstract),
	}
}

func PublishToWatcher(w *modelSummaryWatcher, model string, data interface{}) {
	w.pubsubHandler(model, data)
}

func ModelAccessWatcherMatch(w *modelAccessWatcher, model string) bool {
	return w.match(model)
}

func RunModelAccessWatcher(w *modelAccessWatcher) {
	go w.loop()
}
