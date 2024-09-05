// Copyright 2024 Canonical.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
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

func (j *JIMM) ListApplicationOfferUsers(ctx context.Context, offer names.ApplicationOfferTag, user *dbmodel.Identity, accessLevel string) ([]jujuparams.OfferUserDetails, error) {
	return j.listApplicationOfferUsers(ctx, offer, user, accessLevel)
}

func (j *JIMM) ParseAndValidateTag(ctx context.Context, key string) (*ofganames.Tag, error) {
	return j.parseAndValidateTag(ctx, key)
}

func (j *JIMM) GetUser(ctx context.Context, identifier string) (*openfga.User, error) {
	return j.getUser(ctx, identifier)
}

func (j *JIMM) UpdateUserLastLogin(ctx context.Context, identifier string) error {
	return j.updateUserLastLogin(ctx, identifier)
}

func (j *JIMM) EveryoneUser() *openfga.User {
	return j.everyoneUser()
}
