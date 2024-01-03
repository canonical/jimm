// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
)

var (
	DetermineAccessLevelAfterGrant = determineAccessLevelAfterGrant
	PollDuration                   = pollDuration
	CalculateNextPollDuration      = calculateNextPollDuration
	NewControllerClient            = &newControllerClient
	FillMigrationTarget            = fillMigrationTarget
	InitiateMigration              = &initiateMigration
	ResolveTag                     = resolveTag
)

func WatchController(w *Watcher, ctx context.Context, ctl *dbmodel.Controller) error {
	return w.watchController(ctx, ctl)
}

func NewWatcherWithControllerUnavailableChan(db db.Database, dialer Dialer, pubsub Publisher, testChannel chan error) *Watcher {
	return &Watcher{
		Pubsub:                    pubsub,
		Database:                  db,
		Dialer:                    dialer,
		controllerUnavailableChan: testChannel,
	}
}

func NewWatcherWithDeltaProcessedChannel(db db.Database, dialer Dialer, pubsub Publisher, testChannel chan bool) *Watcher {
	return &Watcher{
		Pubsub:             pubsub,
		Database:           db,
		Dialer:             dialer,
		deltaProcessedChan: testChannel,
	}
}

func (j *JIMM) ListApplicationOfferUsers(ctx context.Context, offer names.ApplicationOfferTag, user *dbmodel.User, accessLevel string) ([]jujuparams.OfferUserDetails, error) {
	return j.listApplicationOfferUsers(ctx, offer, user, accessLevel)
}
