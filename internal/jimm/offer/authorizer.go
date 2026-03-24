// Copyright 2025 Canonical.

package offer

import (
	"context"
	"database/sql"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type OfferAuthorizer struct {
	store   *db.Database
	authSvc *openfga.OFGAClient
}

// NewOfferAuthorizer returns a new OfferAuthorizer that provides methods to
// check if a user is a consumer of an application offer.
func NewOfferAuthorizer(store *db.Database, authSvc *openfga.OFGAClient) (*OfferAuthorizer, error) {
	if store == nil {
		return nil, errors.New("group store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("group authorisation service cannot be nil")
	}
	return &OfferAuthorizer{store, authSvc}, nil
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
