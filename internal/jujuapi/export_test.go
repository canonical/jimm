// Copyright 2025 Canonical.

package jujuapi

import (
	"sync"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/openfga"
)

var (
	NewModelAccessWatcher = newModelAccessWatcher
	ModelInfoFromPath     = modelInfoFromPath
	AuditParamsToFilter   = auditParamsToFilter
	AuditLogDefaultLimit  = limitDefault
	AuditLogUpperLimit    = maxLimit
)

func NewModelSummaryWatcher() *modelSummaryWatcher {
	return &modelSummaryWatcher{
		summaries: make(map[string]jujuparams.ModelAbstract),
	}
}

func PublishToWatcher(w *modelSummaryWatcher, model string, data any) {
	w.pubsubHandler(model, data)
}

func ModelAccessWatcherMatch(w *modelAccessWatcher, model string) bool {
	return w.match(model)
}

func RunModelAccessWatcher(w *modelAccessWatcher, wg *sync.WaitGroup) {
	wg.Go(func() {
		w.loop()
	})
}

type ControllerRoot = controllerRoot

func NewControllerRoot(j JIMM, p Params) *ControllerRoot {
	return newControllerRoot(j, p, "")
}

var SetUser = func(r *controllerRoot, u *openfga.User) {
	r.mu.Lock()
	r.user = u
	r.mu.Unlock()
}
