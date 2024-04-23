// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

// AddApplicationOfferParams holds parameters for the Offer method.
type AddApplicationOfferParams struct {
	ModelTag               names.ModelTag
	OwnerTag               names.UserTag
	OfferName              string
	ApplicationName        string
	ApplicationDescription string
	Endpoints              map[string]string
}

// Offer creates a new application offer.
func (j *JIMM) Offer(ctx context.Context, user *openfga.User, offer AddApplicationOfferParams) error {
	const op = errors.Op("jimm.Offer")

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: offer.ModelTag.Id(),
			Valid:  true,
		},
	}
	if err := j.Database.GetModel(ctx, &model); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "model not found")
		}
		return errors.E(op, err)
	}

	isAdmin, err := openfga.IsAdministrator(ctx, user, model.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed administraor check", zap.Error(err))
		return errors.E(op, "failed administrator check", err)
	}
	if !isAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	offerURL := crossmodel.OfferURL{
		User:      model.OwnerIdentityName,
		ModelName: model.Name,
		// Confusingly the application name in the offer URL is
		// actually the offer name.
		ApplicationName: offer.OfferName,
	}

	api, err := j.dial(ctx, &model.Controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	ownerTag := offer.OwnerTag.String()
	if ownerTag == "" {
		ownerTag = user.Tag().String()
	}
	err = api.Offer(ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:               offer.ModelTag.String(),
			OwnerTag:               ownerTag,
			OfferName:              offer.OfferName,
			ApplicationName:        offer.ApplicationName,
			ApplicationDescription: offer.ApplicationDescription,
			Endpoints:              offer.Endpoints,
		})
	if err != nil {
		if strings.Contains(err.Error(), "application offer already exists") {
			return errors.E(op, err, errors.CodeAlreadyExists)
		}
		return errors.E(op, err)
	}

	offerDetails := jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			OfferURL: offerURL.String(),
		},
	}
	err = api.GetApplicationOffer(ctx, &offerDetails)
	if err != nil {
		zapctx.Error(ctx, "failed to fetch details of the created application offer", zaputil.Error(err))
		return errors.E(op, err)
	}

	var doc dbmodel.ApplicationOffer
	doc.FromJujuApplicationOfferAdminDetails(offerDetails)
	if err != nil {
		zapctx.Error(ctx, "failed to convert application offer details", zaputil.Error(err))
		return errors.E(op, err)
	}
	doc.ModelID = model.ID
	err = j.Database.Transaction(func(db *db.Database) error {
		if err := db.AddApplicationOffer(ctx, &doc); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		zapctx.Error(ctx, "failed to store the created application offer", zaputil.Error(err))
		return errors.E(op, err)
	}

	if err := j.OpenFGAClient.AddModelApplicationOffer(
		ctx,
		model.ResourceTag(),
		doc.ResourceTag(),
	); err != nil {
		zapctx.Error(
			ctx,
			"failed to add relation between model and application offer",
			zap.String("model", model.UUID.String),
			zap.String("application-offer", doc.UUID))
	}

	ownerId := offer.OwnerTag.Id()
	if ownerId == "" {
		ownerId = user.Tag().Id()
	}

	identity, err := dbmodel.NewIdentity(ownerId)
	if err != nil {
		return errors.E(op, err)
	}

	owner := openfga.NewUser(
		identity,
		j.OpenFGAClient,
	)
	if err := owner.SetApplicationOfferAccess(ctx, doc.ResourceTag(), ofganames.AdministratorRelation); err != nil {
		zapctx.Error(
			ctx,
			"failed relation between user and application offer",
			zap.String("user", ownerId),
			zap.String("application-offer", doc.UUID))
	}

	everyoneIdentity, err := dbmodel.NewIdentity(ofganames.EveryoneUser)
	if err != nil {
		return errors.E(op, err)
	}

	everyone := openfga.NewUser(
		everyoneIdentity,
		j.OpenFGAClient,
	)
	if err := everyone.SetApplicationOfferAccess(ctx, doc.ResourceTag(), ofganames.ReaderRelation); err != nil {
		zapctx.Error(
			ctx,
			"failed relation between user and application offer",
			zap.String("user", ownerId),
			zap.String("application-offer", doc.UUID))
	}

	return nil
}

// GetApplicationOfferConsumeDetails consume the application offer
// specified by details.ApplicationOfferDetails.OfferURL and completes
// the rest of the details.
func (j *JIMM) GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	const op = errors.Op("jimm.GetApplicationOfferConsumeDetails")

	offer := dbmodel.ApplicationOffer{
		URL: details.Offer.OfferURL,
	}
	if err := j.Database.GetApplicationOffer(ctx, &offer); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "application offer not found")
		}
		return errors.E(op, err)
	}

	accessLevel, err := j.getUserOfferAccess(ctx, user, &offer)
	if err != nil {
		return errors.E(op, err)
	}

	switch accessLevel {
	case string(jujuparams.OfferAdminAccess):
	case string(jujuparams.OfferConsumeAccess):
	case string(jujuparams.OfferReadAccess):
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	default:
		// TODO (ashipika)
		//   - think about the returned error code
		return errors.E(op, errors.CodeNotFound)
	}

	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		permission{
			resource: jimmnames.NewApplicationOfferTag(offer.UUID).String(),
			relation: accessLevel,
		},
	)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	if err := api.GetApplicationOfferConsumeDetails(ctx, user.ResourceTag(), details, v); err != nil {
		return errors.E(op, err)
	}

	// Fix the consume details from the controller to be correct for JAAS.
	// Filter out any juju local users.
	users, err := j.listApplicationOfferUsers(ctx, offer.ResourceTag(), user.Identity, accessLevel)
	if err != nil {
		return errors.E(op, err)
	}
	details.Offer.Users = users

	ci := details.ControllerInfo
	// Fix the addresses to be a controller's external addresses.
	details.ControllerInfo = &jujuparams.ExternalControllerInfo{
		ControllerTag: offer.Model.Controller.Tag().String(),
		Alias:         offer.Model.Controller.Name,
	}
	if offer.Model.Controller.PublicAddress != "" {
		details.ControllerInfo.Addrs = []string{offer.Model.Controller.PublicAddress}
	} else {
		details.ControllerInfo.Addrs = ci.Addrs
		details.ControllerInfo.CACert = ci.CACert
	}

	return nil
}

// listApplicationOfferUsers filters the application offer user list
// to be suitable for the given user at the given access level. All juju-
// local users are omitted, and if the user is not an admin then they can
// only see themselves.
// TODO(Kian) CSS-6040 Consider changing wherever this function is used to
// better encapsulate transforming Postgres/OpenFGA objects into Juju objects.
func (j *JIMM) listApplicationOfferUsers(ctx context.Context, offer names.ApplicationOfferTag, user *dbmodel.Identity, accessLevel string) ([]jujuparams.OfferUserDetails, error) {
	users := make(map[string]string)
	// we loop through relations in a decreasing order of access
	for _, relation := range []openfga.Relation{
		ofganames.AdministratorRelation,
		ofganames.ConsumerRelation,
		ofganames.ReaderRelation,
	} {
		usersWithRelation, err := openfga.ListUsersWithAccess(ctx, j.OpenFGAClient, offer, relation)
		if err != nil {
			return nil, errors.E(err)
		}
		for _, user := range usersWithRelation {
			// if the user is in the users map, it must already have a higher
			// access level - we skip this user
			if users[user.Name] != "" {
				continue
			}
			users[user.Name] = ToOfferAccessString(relation)
		}
	}

	userDetails := []jujuparams.OfferUserDetails{}
	for username, level := range users {
		// non-admin users should only see their own access level
		// and the access level of "everyone" - meaning the access
		// level everybody has.
		if accessLevel != string(jujuparams.OfferAdminAccess) && username != ofganames.EveryoneUser && username != user.Name {
			continue
		}
		userDetails = append(userDetails, jujuparams.OfferUserDetails{
			UserName: username,
			Access:   level,
			// TODO (alesstimec) this is missing the DisplayName - we could
			// fetch it from the DB if it is REALLY important.
		})
	}
	return userDetails, nil
}

// GetApplicationOffer returns details of the offer with the specified URL.
func (j *JIMM) GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jimm.GetApplicationOffer")

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err := j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, errors.E(op, err, "application offer not found")
		}
		return nil, errors.E(op, err)
	}

	accessLevel, err := j.getUserOfferAccess(ctx, user, &offer)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// if this user does not have access to this application offer
	// we return a not found error.
	if accessLevel == "" {
		return nil, errors.E(op, errors.CodeNotFound, "application offer not found")
	}

	// Always collect application-offer admin details from the
	// controller. The all-watcher events do not include enough
	// information to reasonably keep the local database up-to-date,
	// and it would be non-trivial to make it do so.
	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		permission{
			resource: jimmnames.NewApplicationOfferTag(offer.UUID).String(),
			relation: accessLevel,
		},
	)
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()

	var offerDetails jujuparams.ApplicationOfferAdminDetails
	offerDetails.OfferURL = offerURL
	if err := api.GetApplicationOffer(ctx, &offerDetails); err != nil {
		return nil, errors.E(op, err)
	}

	if accessLevel != string(jujuparams.OfferAdminAccess) {
		offerDetails.Connections = nil
	}
	users, err := j.listApplicationOfferUsers(ctx, offer.ResourceTag(), user.Identity, accessLevel)
	if err != nil {
		return nil, errors.E(op, err)
	}
	offerDetails.Users = users

	return &offerDetails, nil
}

// GrantOfferAccess grants rights for an application offer.
func (j *JIMM) GrantOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
	const op = errors.Op("jimm.GrantOfferAccess")

	identity, err := dbmodel.NewIdentity(ut.Id())
	if err != nil {
		return errors.E(op, err)
	}

	err = j.doApplicationOfferAdmin(ctx, user, offerURL, func(offer *dbmodel.ApplicationOffer, api API) error {
		tUser := openfga.NewUser(identity, j.OpenFGAClient)
		currentRelation := tUser.GetApplicationOfferAccess(ctx, offer.ResourceTag())
		currentAccessLevel := ToOfferAccessString(currentRelation)
		targetAccessLevel := determineAccessLevelAfterGrant(currentAccessLevel, string(access))

		// NOTE (alesstimec) not removing the current access level as it might be an
		// indirect relation.
		if targetAccessLevel != currentAccessLevel {
			relation, err := ToOfferRelation(targetAccessLevel)
			if err != nil {
				return errors.E(op, err)
			}
			err = tUser.SetApplicationOfferAccess(ctx, offer.ResourceTag(), relation)
			if err != nil {
				return errors.E(op, err)
			}
		}

		return nil
	})

	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func determineAccessLevelAfterGrant(currentAccessLevel, grantAccessLevel string) string {
	switch currentAccessLevel {
	case string(jujuparams.OfferAdminAccess):
		return string(jujuparams.OfferAdminAccess)
	case string(jujuparams.OfferConsumeAccess):
		switch grantAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferAdminAccess)
		default:
			return string(jujuparams.OfferConsumeAccess)
		}
	case string(jujuparams.OfferReadAccess):
		switch grantAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferAdminAccess)
		case string(jujuparams.OfferConsumeAccess):
			return string(jujuparams.OfferConsumeAccess)
		default:
			return string(jujuparams.OfferReadAccess)
		}
	default:
		return grantAccessLevel
	}
}

// RevokeOfferAccess revokes rights for an application offer.
func (j *JIMM) RevokeOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error) {
	const op = errors.Op("jimm.RevokeOfferAccess")

	identity, err := dbmodel.NewIdentity(ut.Id())
	if err != nil {
		return errors.E(op, err)
	}

	err = j.doApplicationOfferAdmin(ctx, user, offerURL, func(offer *dbmodel.ApplicationOffer, api API) error {
		tUser := openfga.NewUser(identity, j.OpenFGAClient)
		targetRelation, err := ToOfferRelation(string(access))
		if err != nil {
			return errors.E(op, err)
		}
		err = tUser.UnsetApplicationOfferAccess(ctx, offer.ResourceTag(), targetRelation, false)
		if err != nil {
			return errors.E(op, err, "failed to unset given access")
		}

		// Checking if the target user still has the given access to the
		// application offer (which is possible because of indirect relations),
		// and if so, returning an informative error.
		currentRelation := tUser.GetApplicationOfferAccess(ctx, offer.ResourceTag())
		stillHasAccess := false
		switch targetRelation {
		case ofganames.AdministratorRelation:
			switch currentRelation {
			case ofganames.AdministratorRelation:
				stillHasAccess = true
			}
		case ofganames.ConsumerRelation:
			switch currentRelation {
			case ofganames.AdministratorRelation, ofganames.ConsumerRelation:
				stillHasAccess = true
			}
		case ofganames.ReaderRelation:
			switch currentRelation {
			case ofganames.AdministratorRelation, ofganames.ConsumerRelation, ofganames.ReaderRelation:
				stillHasAccess = true
			}
		}

		if stillHasAccess {
			return errors.E(op, "unable to completely revoke given access due to other relations; try to remove them as well, or use 'jimmctl' for more control")
		}
		return nil
	})

	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// DestroyOffer removes the application offer.
func (j *JIMM) DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error {
	const op = errors.Op("jimm.DestroyOffer")

	err := j.doApplicationOfferAdmin(ctx, user, offerURL, func(offer *dbmodel.ApplicationOffer, api API) error {
		if err := api.DestroyApplicationOffer(ctx, offerURL, force); err != nil {
			return err
		}
		if err := j.Database.DeleteApplicationOffer(ctx, offer); err != nil {
			return err
		}
		if err := j.OpenFGAClient.RemoveApplicationOffer(
			ctx,
			offer.ResourceTag(),
		); err != nil {
			zapctx.Error(
				ctx,
				"cannot remove application offer",
				zap.String("application-offer", offer.UUID))
		}

		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// UpdateApplicationOffer fetches offer details from the controller and updates the
// application offer in JIMM DB.
func (j *JIMM) UpdateApplicationOffer(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error {
	const op = errors.Op("jimm.UpdateApplicationOffer")

	offer := dbmodel.ApplicationOffer{
		UUID: offerUUID,
	}

	err := j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "application offer not found")
		}
		return errors.E(op, err)
	}

	if removed {
		err := j.Database.DeleteApplicationOffer(ctx, &offer)
		if err != nil {
			return errors.E(op, err)
		}
		return nil
	}

	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		permission{
			resource: jimmnames.NewApplicationOfferTag(offer.UUID).String(),
			relation: string(jujuparams.OfferAdminAccess),
		},
	)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	offerDetails := jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			OfferURL: offer.URL,
		},
	}
	err = api.GetApplicationOffer(ctx, &offerDetails)
	if err != nil {
		return errors.E(op, err)
	}

	offer.FromJujuApplicationOfferAdminDetails(offerDetails)
	err = j.Database.UpdateApplicationOffer(ctx, &offer)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// getUserOfferAccess returns the access level string for the user to the
// application offer. It returns the highest access level the user is granted.
func (j *JIMM) getUserOfferAccess(ctx context.Context, user *openfga.User, offer *dbmodel.ApplicationOffer) (string, error) {
	isOfferAdmin, err := openfga.IsAdministrator(ctx, user, offer.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return "", errors.E(err)
	}
	if isOfferAdmin {
		return string(jujuparams.OfferAdminAccess), nil
	}
	isOfferConsumer, err := user.IsApplicationOfferConsumer(ctx, offer.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return "", errors.E(err)
	}
	if isOfferConsumer {
		return string(jujuparams.OfferConsumeAccess), nil
	}
	isOfferReader, err := user.IsApplicationOfferReader(ctx, offer.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return "", errors.E(err)
	}
	if isOfferReader {
		return string(jujuparams.OfferReadAccess), nil
	}
	return "", nil
}

// FindApplicationOffers returns details of offers matching the specified filter.
func (j *JIMM) FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jimm.FindApplicationOffers")

	if len(filters) == 0 {
		return nil, errors.E(op, errors.CodeBadRequest, "at least one filter must be specified")
	}

	offerFilters, err := j.applicationOfferFilters(ctx, filters...)
	if err != nil {
		return nil, errors.E(op, err)
	}
	userOfferUUIDs, err := user.ListApplicationOffers(ctx, ofganames.ReaderRelation)
	if err != nil {
		return nil, errors.E(op, err)
	}
	offerFilters = append(offerFilters, db.ApplicationOfferFilterByUUID(userOfferUUIDs))
	offers, err := j.Database.FindApplicationOffers(ctx, offerFilters...)
	if err != nil {
		return nil, errors.E(op, err)
	}
	offerDetails := make([]jujuparams.ApplicationOfferAdminDetails, len(offers))
	for i, offer := range offers {
		// TODO (alesstimec) Optimize this: currently check all possible
		// permission levels for an offer, this is suboptimal.
		accessLevel, err := j.getUserOfferAccess(ctx, user, &offer)
		if err != nil {
			return nil, errors.E(op, err)
		}

		offerDetails[i] = offer.ToJujuApplicationOfferDetails()

		// non-admin users should not see connections of an application
		// offer.
		if accessLevel != "admin" {
			offerDetails[i].Connections = nil
		}
		users, err := j.listApplicationOfferUsers(ctx, offer.ResourceTag(), user.Identity, accessLevel)
		if err != nil {
			return nil, errors.E(op, err)
		}
		offerDetails[i].Users = users
	}
	return offerDetails, nil
}

func (j *JIMM) applicationOfferFilters(ctx context.Context, jujuFilters ...jujuparams.OfferFilter) ([]db.ApplicationOfferFilter, error) {
	filters := []db.ApplicationOfferFilter{}
	for _, f := range jujuFilters {
		if f.ModelName != "" {
			filters = append(
				filters,
				db.ApplicationOfferFilterByModel(f.ModelName),
			)
		}
		if f.ApplicationName != "" {
			filters = append(
				filters,
				db.ApplicationOfferFilterByApplication(f.ApplicationName),
			)
		}
		if f.OfferName != "" {
			filters = append(
				filters,
				db.ApplicationOfferFilterByName(f.OfferName),
			)
		}
		if f.ApplicationDescription != "" {
			filters = append(
				filters,
				db.ApplicationOfferFilterByDescription(f.ApplicationDescription),
			)
		}
		if len(f.Endpoints) > 0 {
			for _, ep := range f.Endpoints {
				filters = append(
					filters,
					db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
						Interface: ep.Interface,
						Name:      ep.Name,
						Role:      string(ep.Role),
					}),
				)
			}
		}
		if len(f.AllowedConsumerTags) > 0 {
			for _, u := range f.AllowedConsumerTags {
				identity, err := dbmodel.NewIdentity(u)
				if err != nil {
					return nil, errors.E(err)
				}

				ofgaUser := openfga.NewUser(identity, j.OpenFGAClient)
				offerUUIDs, err := ofgaUser.ListApplicationOffers(ctx, ofganames.ConsumerRelation)
				if err != nil {
					return nil, errors.E(err)
				}
				filters = append(
					filters,
					db.ApplicationOfferFilterByUUID(offerUUIDs),
				)
			}
		}

	}
	return filters, nil
}

// ListApplicationOffers returns details of offers matching the specified filter.
func (j *JIMM) ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jimm.ListApplicationOffers")

	type modelKey struct {
		name          string
		ownerUsername string
	}

	if len(filters) == 0 {
		return nil, errors.E(op, errors.CodeBadRequest, "at least one filter must be specified")
	}

	modelFilters := make(map[modelKey][]jujuparams.OfferFilter)
	for _, f := range filters {
		if f.ModelName == "" {
			return nil, errors.E(op, "application offer filter must specify a model name")
		}
		if f.OwnerName == "" {
			f.OwnerName = user.Name
		}
		m := modelKey{
			name:          f.ModelName,
			ownerUsername: f.OwnerName,
		}
		modelFilters[m] = append(modelFilters[m], f)
	}

	var offers []jujuparams.ApplicationOfferAdminDetails

	var keys []modelKey
	for k := range modelFilters {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ownerUsername == keys[j].ownerUsername {
			return keys[i].name < keys[j].name
		}
		return keys[i].ownerUsername < keys[j].ownerUsername
	})

	for _, k := range keys {
		m := dbmodel.Model{
			Name:              k.name,
			OwnerIdentityName: k.ownerUsername,
		}
		offerDetails, err := j.listApplicationOffersForModel(ctx, user, &m, modelFilters[k])
		if err != nil {
			return nil, errors.E(op, err)
		}
		for _, offer := range offerDetails {
			users, err := j.listApplicationOfferUsers(ctx, names.NewApplicationOfferTag(offer.OfferUUID), user.Identity, "admin")
			if err != nil {
				return nil, errors.E(op, err)
			}
			offer.Users = users
			offers = append(offers, offer)
		}
	}
	return offers, nil
}

func (j *JIMM) listApplicationOffersForModel(ctx context.Context, user *openfga.User, m *dbmodel.Model, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jimm.listApplicationOffersForModel")

	if err := j.Database.GetModel(ctx, m); err != nil {
		return nil, errors.E(op, err)
	}
	isModelAdmin, err := openfga.IsAdministrator(ctx, user, m.ResourceTag())
	if err != nil {
		return nil, errors.E(op, err)
	}
	if !user.JimmAdmin && !isModelAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	api, err := j.dial(ctx, &m.Controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()
	offers, err := api.ListApplicationOffers(ctx, filters)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return offers, nil
}

// doApplicationOfferAdmin performs the given function on an application offer
// only if the given user has admin access on the model of the offer, or is a
// controller superuser. Otherwise an unauthorized error is returned.
//
// Note: The user does not need to have any access level on the offer itself.
// As long as they are model admins or controller superusers they can also
// manipulate the application offer as admins.
func (j *JIMM) doApplicationOfferAdmin(ctx context.Context, user *openfga.User, offerURL string, f func(offer *dbmodel.ApplicationOffer, api API) error) error {
	const op = errors.Op("jimm.doApplicationOfferAdmin")

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	if err := j.Database.GetApplicationOffer(ctx, &offer); err != nil {
		return errors.E(op, err)
	}

	isOfferAdmin, err := openfga.IsAdministrator(ctx, user, offer.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isOfferAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	// add offer admin claim
	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		permission{
			resource: jimmnames.NewApplicationOfferTag(offer.UUID).String(),
			relation: string(jujuparams.OfferAdminAccess),
		},
	)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	if err := f(&offer, api); err != nil {
		return errors.E(op, err)
	}
	return nil
}
