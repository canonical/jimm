// Copyright 2024 Canonical.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
)

var (
	PollDuration              = pollDuration
	CalculateNextPollDuration = calculateNextPollDuration
	NewControllerClient       = &newControllerClient
	FillMigrationTarget       = fillMigrationTarget
	InitiateMigration         = &initiateMigration
)

func NewWatcherWithControllerUnavailableChan(db *db.Database, dialer Dialer, pubsub Publisher, testChannel chan error) *Watcher {
	return &Watcher{
		Pubsub:                    pubsub,
		Database:                  db,
		Dialer:                    dialer,
		controllerUnavailableChan: testChannel,
	}
}

func NewWatcherWithDeltaProcessedChannel(db *db.Database, dialer Dialer, pubsub Publisher, testChannel chan bool) *Watcher {
	return &Watcher{
		Pubsub:             pubsub,
		Database:           db,
		Dialer:             dialer,
		deltaProcessedChan: testChannel,
	}
}

func (j *JIMM) ListApplicationOfferUsers(ctx context.Context, offer names.ApplicationOfferTag, user *dbmodel.Identity, adminAccess bool) ([]jujuparams.OfferUserDetails, error) {
	return j.listApplicationOfferUsers(ctx, offer, user, adminAccess)
}

func (j *JIMM) EveryoneUser() *openfga.User {
	return j.everyoneUser()
}
