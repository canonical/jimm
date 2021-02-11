// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
)

// AddApplicationOfferParams holds parameters for the Offer method.
type AddApplicationOfferParams struct {
	ModelTag               names.ModelTag
	OfferName              string
	ApplicationName        string
	ApplicationDescription string
	Endpoints              map[string]string
}

// Offer creates a new application offer.
func (j *JIMM) Offer(ctx context.Context, user *dbmodel.User, offer AddApplicationOfferParams) (err error) {
	const op = errors.Op("jimm.Offer")
	model := dbmodel.Model{
		UUID: sql.NullString{
			String: offer.ModelTag.Id(),
			Valid:  true,
		},
	}
	if err := j.Database.GetModel(ctx, &model, db.AssociatedApplications()); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "model not found")
		}
		return errors.E(op, err)
	}

	var userAccessLevel string
	for _, u := range model.Users {
		if u.ID == user.ID {
			userAccessLevel = u.Access
			break
		}
	}
	if userAccessLevel != string(jujuparams.ModelAdminAccess) {
		return errors.E(op, errors.CodeUnauthorized)
	}

	var application *dbmodel.Application
	for _, a := range model.Applications {
		if a.Name == offer.ApplicationName {
			application = &a
			break
		}
	}
	if application == nil {
		return errors.E(op, errors.CodeNotFound, "application not found")
	}

	for _, existingOffer := range application.Offers {
		if offer.OfferName == existingOffer.Name {
			return errors.E(op, errors.CodeAlreadyExists, "application offer already exists")
		}
	}

	api, err := j.dial(ctx, &model.Controller, offer.ModelTag)
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	err = api.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:               offer.ModelTag.String(),
		OfferName:              offer.OfferName,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
		Endpoints:              offer.Endpoints,
	})
	if err != nil {
		if apiconn.IsAPIError(err) && strings.Contains(err.Error(), "application offer already exists") {
			return errors.E(op, err, errors.CodeAlreadyExists)
		}
		return errors.E(op, err)
	}

	offerURL := crossmodel.OfferURL{
		User:            user.Username,
		ModelName:       model.Name,
		ApplicationName: offer.ApplicationName,
	}

	// Ensure the user creating the offer is an admin for the offer.
	if err := api.GrantApplicationOfferAccess(ctx, offerURL.String(), names.NewUserTag(user.Username), jujuparams.OfferAdminAccess); err != nil {
		// TODO (ashipika) we could remove the offer from the controller, if we fail to grant
		// access to it
		zapctx.Error(ctx, "failed to grant application offer access to user", zaputil.Error(err))
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
	doc.FromJujuApplicationOfferAdminDetails(application, offerDetails)
	if err != nil {
		zapctx.Error(ctx, "failed to convert application offer details", zaputil.Error(err))
		return errors.E(op, err)
	}

	err = j.Database.AddApplicationOffer(ctx, &doc)
	if err != nil {
		zapctx.Error(ctx, "failed to store the created application offer", zaputil.Error(err))
		return errors.E(op, err)
	}

	return nil
}

// GetApplicationOfferConsumeDetails consume the application offer
// specified by details.ApplicationOfferDetails.OfferURL and completes
// the rest of the details.
func (j *JIMM) GetApplicationOfferConsumeDetails(ctx context.Context, user *dbmodel.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
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

	var accessLevel string
	for _, u := range offer.Users {
		if u.UserID == user.ID {
			accessLevel = u.Access
			break
		}
	}
	if accessLevel == "" {
		// TODO (ashipika)
		//   - think about the returned error code
		//   - how do we model public application offers
		//     (accessible to everybody)
		return errors.E(op, errors.CodeNotFound)
	}

	switch accessLevel {
	case string(jujuparams.OfferAdminAccess):
	case string(jujuparams.OfferConsumeAccess):
	case string(jujuparams.OfferReadAccess):
		return errors.E(op, errors.CodeUnauthorized)
	default:
		return errors.E(op, errors.CodeNotFound)
	}

	model := dbmodel.Model{
		ID: offer.Application.ModelID,
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return errors.E(op, err)
	}

	controller := model.Controller
	api, err := j.dial(ctx, &controller, names.NewModelTag(model.UUID.String))
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	if err := api.GetApplicationOfferConsumeDetails(ctx, names.NewUserTag(user.Username), details, v); err != nil {
		return errors.E(op, err)
	}

	// Fix the consume details from the controller to be correct for JAAS.
	// Filter out any juju local users.
	details.Offer.Users = filterApplicationOfferUsers(user, accessLevel, details.Offer.Users)

	// Fix the addresses to be a controller's external addresses.
	details.ControllerInfo = &jujuparams.ExternalControllerInfo{
		ControllerTag: model.Controller.Tag().String(),
		Alias:         model.Controller.Name,
		CACert:        model.Controller.CACertificate,
	}
	details.ControllerInfo.Addrs = append(details.ControllerInfo.Addrs, model.Controller.PublicAddress)

	return nil
}

// filterApplicationOfferUsers filters the application offer user list
// to be suitable for the given user at the given access level. All juju-
// local users are omitted, and if the user is not an admin then they can
// only see themselves.
func filterApplicationOfferUsers(user *dbmodel.User, accessLevel string, users []jujuparams.OfferUserDetails) []jujuparams.OfferUserDetails {
	filtered := make([]jujuparams.OfferUserDetails, 0, len(users))
	for _, u := range users {
		if accessLevel == string(jujuparams.OfferAdminAccess) || u.UserName == user.Username {
			filtered = append(filtered, u)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UserName < filtered[j].UserName
	})
	return filtered
}

// GetApplicationOffer returns details of the offer with the specified URL.
func (j *JIMM) GetApplicationOffer(ctx context.Context, user *dbmodel.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetails, error) {
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

	var accessLevel string
	for _, userAccess := range offer.Users {
		if userAccess.UserID == user.ID {
			accessLevel = userAccess.Access
			break
		}
	}

	if accessLevel == "" {
		for _, userAccess := range offer.Users {
			if userAccess.User.Username == "everyone@external" {
				accessLevel = userAccess.Access
				break
			}
		}
	}
	if accessLevel == "" {
		return nil, errors.E(op, errors.CodeNotFound, "application offer not found")
	}

	offerDetails := offer.ToJujuApplicationOfferDetails(user, accessLevel)

	return offerDetails, nil
}

// GrantOfferAccess grants rights for an application offer.
func (j *JIMM) GrantOfferAccess(ctx context.Context, user, offerUser *dbmodel.User, offerURL string, access jujuparams.OfferAccessPermission) (err error) {
	const op = errors.Op("jimm.GrantOfferAccess")

	// first we need to fetch the offer to get it's UUID
	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err = j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "application offer not found")
		}
		return errors.E(op, err)
	}

	var accessLevel string
	for _, ua := range offer.Users {
		if ua.UserID == user.ID {
			accessLevel = ua.Access
			break
		}
	}

	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch accessLevel {
	case "":
		return errors.E(errors.CodeNotFound)
	case string(jujuparams.OfferReadAccess), string(jujuparams.OfferConsumeAccess):
		return errors.E(errors.CodeUnauthorized)
	default:
	}

	model := dbmodel.Model{
		ID: offer.Application.ModelID,
	}
	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return errors.E(op, err)
	}

	// grant access on the controller
	api, err := j.dial(ctx, &model.Controller, model.Tag().(names.ModelTag))
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	err = api.GrantApplicationOfferAccess(ctx, offer.URL, offerUser.Tag().(names.UserTag), access)
	if err != nil {
		return errors.E(op, err)
	}

	found := false
	for i, ua := range offer.Users {
		if ua.User.ID == offerUser.ID {
			found = true
			offer.Users[i].Access = determineAccessLevelAfterGrant(ua.Access, string(access))
		}
	}
	if !found {
		offer.Users = append(offer.Users, dbmodel.UserApplicationOfferAccess{
			UserID: offerUser.ID,
			User:   *offerUser,
			Access: string(access),
		})
	}

	err = j.Database.UpdateApplicationOffer(ctx, &offer)
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
func (j *JIMM) RevokeOfferAccess(ctx context.Context, user, offerUser *dbmodel.User, offerURL string, access jujuparams.OfferAccessPermission) (err error) {
	const op = errors.Op("jimm.RevokeOfferAccess")

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err = j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "application offer not found")
		}
		return errors.E(op, err)
	}

	var authenticatedUserAccessLevel string
	for _, ua := range offer.Users {
		if ua.UserID == user.ID {
			authenticatedUserAccessLevel = ua.Access
			break
		}
	}
	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch authenticatedUserAccessLevel {
	case "":
		return errors.E(op, errors.CodeNotFound)
	case string(jujuparams.OfferReadAccess), string(jujuparams.OfferConsumeAccess):
		return errors.E(op, errors.CodeUnauthorized)
	default:
	}

	var userAccessLevel string
	updateNeeded := false
	for i, ua := range offer.Users {
		if ua.UserID == offerUser.ID {
			userAccessLevel = determineAccessLevelAfterRevoke(ua.Access, string(access))
			offer.Users[i].Access = userAccessLevel
			updateNeeded = true
			break
		}
	}

	model := dbmodel.Model{
		ID: offer.Application.ModelID,
	}

	// then revoke on the actual controller
	api, err := j.dial(ctx, &model.Controller, model.Tag().(names.ModelTag))
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	err = api.RevokeApplicationOfferAccess(ctx, offer.URL, offerUser.Tag().(names.UserTag), access)
	if err != nil {
		return errors.E(op, err)
	}

	if updateNeeded {
		err = j.Database.UpdateApplicationOffer(ctx, &offer)
		if err != nil {
			return errors.E(op, err)
		}
	}

	return nil
}

func determineAccessLevelAfterRevoke(currentAccessLevel, revokeAccessLevel string) string {
	switch currentAccessLevel {
	case string(jujuparams.OfferAdminAccess):
		switch revokeAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferConsumeAccess)
		case string(jujuparams.OfferConsumeAccess):
			return string(jujuparams.OfferReadAccess)
		case string(jujuparams.OfferReadAccess):
			return ""
		default:
			return ""
		}
	case string(jujuparams.OfferConsumeAccess):
		switch revokeAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferConsumeAccess)
		case string(jujuparams.OfferConsumeAccess):
			return string(jujuparams.OfferReadAccess)
		case string(jujuparams.OfferReadAccess):
			return ""
		default:
			return ""
		}
	case string(jujuparams.OfferReadAccess):
		switch revokeAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferReadAccess)
		case string(jujuparams.OfferConsumeAccess):
			return string(jujuparams.OfferReadAccess)
		case string(jujuparams.OfferReadAccess):
			return ""
		default:
			return ""
		}
	default:
		return ""
	}
}
