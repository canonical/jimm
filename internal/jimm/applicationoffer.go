// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"

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
func (j *JIMM) Offer(ctx context.Context, user *dbmodel.User, offer AddApplicationOfferParams) error {
	const op = errors.Op("jimm.Offer")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: user.Tag().String(),
		Action:  "create",
		Params: dbmodel.StringMap{
			"model":       offer.ModelTag.String(),
			"name":        offer.OfferName,
			"application": offer.ApplicationName,
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: offer.ModelTag.Id(),
			Valid:  true,
		},
	}
	if err := j.Database.GetModel(ctx, &model, db.AssociatedApplications()); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return fail(errors.E(op, err, "model not found"))
		}
		return fail(errors.E(op, err))
	}

	var userAccessLevel string
	for _, u := range model.Users {
		if u.ID == user.ID {
			userAccessLevel = u.Access
			break
		}
	}
	if userAccessLevel != string(jujuparams.ModelAdminAccess) {
		return fail(errors.E(op, errors.CodeUnauthorized))
	}

	var application *dbmodel.Application
	for _, a := range model.Applications {
		if a.Name == offer.ApplicationName {
			application = &a
			break
		}
	}
	if application == nil {
		return fail(errors.E(op, errors.CodeNotFound, "application not found"))
	}

	for _, existingOffer := range application.Offers {
		if offer.OfferName == existingOffer.Name {
			return fail(errors.E(op, errors.CodeAlreadyExists, "application offer already exists"))
		}
	}

	api, err := j.dial(ctx, &model.Controller, names.ModelTag{})
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
			return fail(errors.E(op, err, errors.CodeAlreadyExists))
		}
		return fail(errors.E(op, err))
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
		return fail(errors.E(op, err))
	}

	offerDetails := jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			OfferURL: offerURL.String(),
		},
	}
	err = api.GetApplicationOffer(ctx, &offerDetails)
	if err != nil {
		zapctx.Error(ctx, "failed to fetch details of the created application offer", zaputil.Error(err))
		return fail(errors.E(op, err))
	}

	var doc dbmodel.ApplicationOffer
	doc.FromJujuApplicationOfferAdminDetails(application, offerDetails)
	if err != nil {
		zapctx.Error(ctx, "failed to convert application offer details", zaputil.Error(err))
		return fail(errors.E(op, err))
	}

	err = j.Database.AddApplicationOffer(ctx, &doc)
	if err != nil {
		zapctx.Error(ctx, "failed to store the created application offer", zaputil.Error(err))
		return fail(errors.E(op, err))
	}

	ale.Success = true
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

	accessLevel := offer.UserAccessLevel(user.Username)
	if accessLevel == "" {
		accessLevel = offer.UserAccessLevel("everyone@external")
	}

	if accessLevel == "" {
		// TODO (ashipika)
		//   - think about the returned error code
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

	model := offer.Application.Model
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

	accessLevel := offer.UserAccessLevel(user.Username)
	if accessLevel == "" {
		accessLevel = offer.UserAccessLevel("everyone@external")
	}
	if accessLevel == "" {
		return nil, errors.E(op, errors.CodeNotFound, "application offer not found")
	}

	offerDetails := offer.ToJujuApplicationOfferDetails()
	offerDetails = filterApplicationOfferDetail(offerDetails, user, accessLevel)

	return &offerDetails, nil
}

func filterApplicationOfferDetail(offerDetails jujuparams.ApplicationOfferAdminDetails, user *dbmodel.User, accessLevel string) jujuparams.ApplicationOfferAdminDetails {
	offer := offerDetails
	if accessLevel != string(jujuparams.OfferAdminAccess) {
		offer.Connections = nil
	}
	if accessLevel != string(jujuparams.OfferAdminAccess) {
		var users []jujuparams.OfferUserDetails
		for _, ua := range offerDetails.Users {
			if ua.UserName == user.Username {
				users = append(users, ua)
			}
		}
		offer.Users = users
	}
	return offer
}

// GrantOfferAccess grants rights for an application offer.
func (j *JIMM) GrantOfferAccess(ctx context.Context, user, offerUser *dbmodel.User, offerURL string, access jujuparams.OfferAccessPermission) error {
	const op = errors.Op("jimm.GrantOfferAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: user.Tag().String(),
		Action:  "grant",
		Params: dbmodel.StringMap{
			"url":    offerURL,
			"user":   offerUser.Tag().String(),
			"access": string(access),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	// first we need to fetch the offer to get it's UUID
	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err := j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return fail(errors.E(op, err, "application offer not found"))
		}
		return fail(errors.E(op, err))
	}
	ale.Tag = offer.Tag().String()

	authenticatedUserAccessLevel := offer.UserAccessLevel(user.Username)

	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch authenticatedUserAccessLevel {
	case "":
		return fail(errors.E(errors.CodeNotFound))
	case string(jujuparams.OfferReadAccess), string(jujuparams.OfferConsumeAccess):
		return fail(errors.E(errors.CodeUnauthorized))
	default:
	}

	model := offer.Application.Model
	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return fail(errors.E(op, err))
	}

	// grant access on the controller
	api, err := j.dial(ctx, &model.Controller, names.ModelTag{})
	if err != nil {
		return fail(errors.E(op, err))
	}
	defer api.Close()

	err = api.GrantApplicationOfferAccess(ctx, offer.URL, offerUser.Tag().(names.UserTag), access)
	if err != nil {
		return fail(errors.E(op, err))
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
		return fail(errors.E(op, err))
	}

	ale.Success = true
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

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: user.Tag().String(),
		Action:  "revoke",
		Params: dbmodel.StringMap{
			"url":    offerURL,
			"user":   offerUser.Tag().String(),
			"access": string(access),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err = j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return fail(errors.E(op, err, "application offer not found"))
		}
		return fail(errors.E(op, err))
	}
	ale.Tag = offer.Tag().String()

	authenticatedUserAccessLevel := offer.UserAccessLevel(user.Username)

	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch authenticatedUserAccessLevel {
	case "":
		return fail(errors.E(op, errors.CodeNotFound))
	case string(jujuparams.OfferReadAccess), string(jujuparams.OfferConsumeAccess):
		return fail(errors.E(op, errors.CodeUnauthorized))
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

	model := offer.Application.Model
	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return fail(errors.E(op, err))
	}

	// then revoke on the actual controller
	api, err := j.dial(ctx, &model.Controller, model.Tag().(names.ModelTag))
	if err != nil {
		return fail(errors.E(op, err))
	}
	defer api.Close()

	err = api.RevokeApplicationOfferAccess(ctx, offer.URL, offerUser.Tag().(names.UserTag), access)
	if err != nil {
		return fail(errors.E(op, err))
	}

	if updateNeeded {
		err = j.Database.UpdateApplicationOffer(ctx, &offer)
		if err != nil {
			return fail(errors.E(op, err))
		}
	}

	ale.Success = true
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

// DestroyOffer removes the application offer.
func (j *JIMM) DestroyOffer(ctx context.Context, user *dbmodel.User, offerURL string, force bool) error {
	const op = errors.Op("jimm.DestroyOffer")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: user.Tag().String(),
		Action:  "destroy",
		Params: dbmodel.StringMap{
			"url":   offerURL,
			"force": strconv.FormatBool(force),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	offer := &dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err := j.Database.GetApplicationOffer(ctx, offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return fail(errors.E(op, err, "application offer not found"))
		}
		return fail(errors.E(op, err))
	}
	ale.Tag = offer.Tag().String()

	accessLevel := offer.UserAccessLevel(user.Username)
	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch accessLevel {
	case "":
		return fail(errors.E(op, errors.CodeNotFound, "application offer not found"))
	case string(jujuparams.OfferReadAccess), string(jujuparams.OfferConsumeAccess):
		return fail(errors.E(op, errors.CodeUnauthorized))
	default:
	}

	model := offer.Application.Model
	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return fail(errors.E(op, err))
	}

	api, err := j.dial(ctx, &model.Controller, model.Tag().(names.ModelTag))
	if err != nil {
		return fail(errors.E(op, err))
	}
	defer api.Close()

	err = api.DestroyApplicationOffer(ctx, offerURL, force)
	if err != nil {
		return fail(errors.E(op, err))
	}

	err = j.Database.DeleteApplicationOffer(ctx, offer)
	if err != nil {
		return fail(errors.E(op, err))
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

	api, err := j.dial(ctx, controller, offer.Application.Model.Tag().(names.ModelTag))
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

	offer.FromJujuApplicationOfferAdminDetails(&offer.Application, offerDetails)
	err = j.Database.UpdateApplicationOffer(ctx, &offer)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FindApplicationOffers returns details of offers matching the specified filter.
func (j *JIMM) FindApplicationOffers(ctx context.Context, user *dbmodel.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jimm.FindApplicationOffers")

	if len(filters) == 0 {
		return nil, errors.E(op, errors.CodeBadRequest, "at least one filter must be specified")
	}

	offerFilters, err := j.applicationOfferFilters(ctx, filters...)
	if err != nil {
		return nil, errors.E(op, err)
	}
	offerFilters = append(offerFilters, db.ApplicationOfferFilterByUser(user.ID))
	offers, err := j.Database.FindApplicationOffers(ctx, offerFilters...)
	if err != nil {
		return nil, errors.E(op, err)
	}
	offerDetails := make([]jujuparams.ApplicationOfferAdminDetails, len(offers))
	for i, offer := range offers {
		accessLevel := offer.UserAccessLevel(user.Username)
		offerDetails[i] = offer.ToJujuApplicationOfferDetails()
		offerDetails[i] = filterApplicationOfferDetail(offerDetails[i], user, accessLevel)
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
				user := dbmodel.User{
					Username: u,
				}
				err := j.Database.GetUser(ctx, &user)
				if err != nil {
					return nil, errors.E(err)
				}
				filters = append(
					filters,
					db.ApplicationOfferFilterByConsumer(user.ID),
				)
			}
		}

	}
	return filters, nil
}
