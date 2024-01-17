// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jujuparams "github.com/juju/juju/rpc/params"
)

var (
	NewModelAccessWatcher = newModelAccessWatcher
	ParseTag              = parseTag
	ResolveTag            = resolveTag
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

func PublishToWatcher(w *modelSummaryWatcher, model string, data interface{}) {
	w.pubsubHandler(model, data)
}

func ModelAccessWatcherMatch(w *modelAccessWatcher, model string) bool {
	return w.match(model)
}

func RunModelAccessWatcher(w *modelAccessWatcher) {
	go w.loop()
}

func ToJAASTag(db db.Database, tag *ofganames.Tag) (string, error) {
	c := controllerRoot{
		jimm: &jimm.JIMM{
			Database: db,
		},
	}
	return c.toJAASTag(context.Background(), tag)
}

func NewControllerRoot(j JIMM, p Params) *controllerRoot {
	return newControllerRoot(j, p)
}

var SetUser = func(r *controllerRoot, u *openfga.User) {
	r.mu.Lock()
	r.user = u
	r.mu.Unlock()
}
