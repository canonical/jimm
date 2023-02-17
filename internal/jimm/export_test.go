// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

var (
	DetermineAccessLevelAfterRevoke = determineAccessLevelAfterRevoke
	DetermineAccessLevelAfterGrant  = determineAccessLevelAfterGrant
)

func (j *JIMM) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	j.addAuditLogEntry(ale)
}

func (w *Watcher) PollControllerModels(ctx context.Context, ctl *dbmodel.Controller) {
	w.pollControllerModels(ctx, ctl)
}

func NewWatcherWithControllerUnavailableChan(db db.Database, dialer Dialer, pubsub Publisher, testChannel chan error) *Watcher {
	return &Watcher{
		Pubsub:                    pubsub,
		Database:                  db,
		Dialer:                    dialer,
		controllerUnavailableChan: testChannel,
	}
}

func (j *JIMM) ListApplicationOfferUsers(ctx context.Context, offer names.ApplicationOfferTag, user *dbmodel.User, accessLevel string) ([]jujuparams.OfferUserDetails, error) {
	return j.listApplicationOfferUsers(ctx, offer, user, accessLevel)
}
