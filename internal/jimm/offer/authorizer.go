// Copyright 2025 Canonical.

package offer

import (
	"context"
	"database/sql"

	"github.com/juju/names/v6"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// IdPGroupFetcher resolves a user's IdP group identifiers without a login
// session, for non-session flows such as macaroon discharge.
//
// Implementations must fail closed: if the groups cannot be resolved,
// return an error; the caller denies access.
type IdPGroupFetcher interface {
	// FetchGroups returns the current IdP group identifiers for the user
	// with the given email/username.
	FetchGroups(ctx context.Context, username string) ([]string, error)
}

type OfferAuthorizer struct {
	store   *db.Database
	authSvc *openfga.OFGAClient
	// idpGroupFetcher resolves the user's IdP groups when no login
	// session is available. May be nil, in which case group-derived
	// access is denied.
	idpGroupFetcher IdPGroupFetcher
}

// NewOfferAuthorizer returns a new OfferAuthorizer that provides methods to
// check if a user is a consumer of an application offer.
func NewOfferAuthorizer(store *db.Database, authSvc *openfga.OFGAClient, idpGroupFetcher IdPGroupFetcher) (*OfferAuthorizer, error) {
	if store == nil {
		return nil, errors.New("group store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("group authorisation service cannot be nil")
	}
	return &OfferAuthorizer{store, authSvc, idpGroupFetcher}, nil
}

// IsUserConsumerForOffer checks if a user is a consumer of an application offer.
// If the user is local, it is mapped to its corresponding external user
// based on the model migration user mapping.
func (offerAuth *OfferAuthorizer) IsUserConsumerForOffer(ctx context.Context, userTag names.UserTag, offerTag names.ApplicationOfferTag) (bool, error) {
	var userIdentifier string
	var err error
	if userTag.IsLocal() {
		userIdentifier, err = offerAuth.resolveLocalUserToExternalUser(ctx, userTag.Id(), offerTag)
		if err != nil {
			return false, err
		}
	} else {
		userIdentifier = userTag.Id()
	}
	identity, err := dbmodel.NewIdentity(userIdentifier)
	if err != nil {
		return false, err
	}
	user := openfga.NewUser(
		identity,
		offerAuth.authSvc,
	)

	// Resolve the user's IdP groups so group-based grants are visible to
	// the check. On error, proceed without groups (fail closed).
	if offerAuth.idpGroupFetcher != nil {
		groups, err := offerAuth.idpGroupFetcher.FetchGroups(ctx, userIdentifier)
		if err != nil {
			zapctx.Error(ctx, "failed to fetch identity groups for discharge check", zap.Error(err), zap.String("user", userIdentifier))
		} else {
			user.SetIDPGroups(groups)
		}
	}

	return user.IsApplicationOfferConsumer(ctx, offerTag)
}

func (offerAuth *OfferAuthorizer) resolveLocalUserToExternalUser(ctx context.Context, localUsername string, offerTag names.ApplicationOfferTag) (string, error) {
	offer := dbmodel.ApplicationOffer{
		UUID: offerTag.Id(),
	}
	err := offerAuth.store.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return "", err
	}
	userMapping := dbmodel.UserMapping{
		ModelUUID: sql.NullString{
			String: offer.Model.UUID.String,
			Valid:  true,
		},
		LocalUser: localUsername,
	}
	err = offerAuth.store.GetUserMapping(ctx, &userMapping)
	if err != nil {
		return "", err
	}
	return userMapping.ExternalUserName, nil
}
