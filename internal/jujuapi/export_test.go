// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/openfga"
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

func ToJAASTag(db db.Database, tag string) (string, error) {
	c := controllerRoot{
		jimm: &jimm.JIMM{
			Database: db,
		},
	}
	return c.toJAASTag(context.Background(), tag)
}

func RemoveRelatedTuples(db db.Database, ofga *openfga.OFGAClient, tag string) error {
	c := controllerRoot{
		jimm: &jimm.JIMM{
			Database: db,
		},
		ofgaClient: ofga,
	}
	return c.removeRelatedTuples(context.Background(), tag)
}
