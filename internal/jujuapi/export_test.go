// Copyright 2024 Canonical.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

var (
	NewModelAccessWatcher = newModelAccessWatcher
	ModelInfoFromPath     = modelInfoFromPath
	AuditParamsToFilter   = auditParamsToFilter
	AuditLogDefaultLimit  = limitDefault
	AuditLogUpperLimit    = maxLimit
)

func HTTPProxier(jimm JIMM) httpProxier {
	return httpProxier{jimm: jimm}
}

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

func ToJAASTag(db db.Database, tag *ofganames.Tag, resolveUUIDs bool) (string, error) {
	jimm := &jimm.JIMM{
		Database: db,
	}
	return jimm.ToJAASTag(context.Background(), tag, resolveUUIDs)
}

func NewControllerRoot(j JIMM, p Params) *controllerRoot {
	return newControllerRoot(j, p, "")
}

func (r *controllerRoot) GetServiceAccount(ctx context.Context, clientID string) (*openfga.User, error) {
	return r.getServiceAccount(ctx, clientID)
}

var SetUser = func(r *controllerRoot, u *openfga.User) {
	r.mu.Lock()
	r.user = u
	r.mu.Unlock()
}
