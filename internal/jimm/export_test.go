// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

var (
	DetermineAccessLevelAfterRevoke = determineAccessLevelAfterRevoke
	DetermineAccessLevelAfterGrant  = determineAccessLevelAfterGrant
	FilterApplicationOfferDetail    = filterApplicationOfferDetail
)

func (j *JIMM) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	j.addAuditLogEntry(ale)
}

func (w *Watcher) PollControllerModels(ctx context.Context, ctl *dbmodel.Controller) {
	w.pollControllerModels(ctx, ctl)
}

func NewWatcherWithDeltaProcessedChannel(db db.Database, dialer Dialer, testChannel chan bool) *Watcher {
	return &Watcher{
		Database:           db,
		Dialer:             dialer,
		deltaProcessedChan: testChannel,
	}
}
