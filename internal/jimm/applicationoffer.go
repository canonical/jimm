// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
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
			"owner":       offer.OwnerTag.String(),
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
	if err := j.Database.GetModel(ctx, &model); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return fail(errors.E(op, err, "model not found"))
		}
		return fail(errors.E(op, err))
	}

	if model.UserAccess(user) != string(jujuparams.ModelAdminAccess) {
		return fail(errors.E(op, errors.CodeUnauthorized, "unauthorized"))
	}

	offerURL := crossmodel.OfferURL{
		User:      model.OwnerUsername,
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
			return fail(errors.E(op, err, errors.CodeAlreadyExists))
		}
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
	doc.FromJujuApplicationOfferAdminDetails(offerDetails)
	if err != nil {
		zapctx.Error(ctx, "failed to convert application offer details", zaputil.Error(err))
		return fail(errors.E(op, err))
	}
	doc.ModelID = model.ID
	err = j.Database.Transaction(func(db *db.Database) error {
		if err := db.AddApplicationOffer(ctx, &doc); err != nil {
			return err
		}
		for _, u := range doc.Users {
			if err := db.UpdateUserApplicationOfferAccess(ctx, &u); err != nil {
				return err
			}
		}
		return nil
	})
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

	accessLevel := offer.UserAccess(user)
	if accessLevel == "" {
		accessLevel = offer.UserAccess(&dbmodel.User{Username: auth.Everyone})
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

	api, err := j.dial(ctx, &offer.Model.Controller, names.ModelTag{})
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

// filterApplicationOfferUsers filters the application offer user list
// to be suitable for the given user at the given access level. All juju-
// local users are omitted, and if the user is not an admin then they can
// only see themselves.
func filterApplicationOfferUsers(user *dbmodel.User, accessLevel string, users []jujuparams.OfferUserDetails) []jujuparams.OfferUserDetails {
	filtered := make([]jujuparams.OfferUserDetails, 0, len(users))
	for _, u := range users {
		// ignore all controller-local users
		if strings.IndexByte(u.UserName, '@') < 0 {
			continue
		}
		if accessLevel == string(jujuparams.OfferAdminAccess) || u.UserName == user.Username || u.UserName == auth.Everyone {
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

	accessLevel := offer.UserAccess(user)
	if accessLevel == "" {
		accessLevel = offer.UserAccess(&dbmodel.User{Username: auth.Everyone})
	}
	if accessLevel == "" {
		return nil, errors.E(op, errors.CodeNotFound, "application offer not found")
	}

	// Always collect application-offer admin details from the
	// controller. The all-watcher events do not include enough
	// information to reasonably keep the local database up-to-date,
	// and it would be non-trivial to make it do so.
	api, err := j.dial(ctx, &offer.Model.Controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()

	var offerDetails jujuparams.ApplicationOfferAdminDetails
	offerDetails.OfferURL = offerURL
	if err := api.GetApplicationOffer(ctx, &offerDetails); err != nil {
		return nil, errors.E(op, err)
	}
	offerDetails = filterApplicationOfferDetail(offerDetails, user, accessLevel)

	return &offerDetails, nil
}

func filterApplicationOfferDetail(offerDetails jujuparams.ApplicationOfferAdminDetails, user *dbmodel.User, accessLevel string) jujuparams.ApplicationOfferAdminDetails {
	offer := offerDetails
	if accessLevel != string(jujuparams.OfferAdminAccess) {
		offer.Connections = nil
	}
	offer.Users = filterApplicationOfferUsers(user, accessLevel, offer.Users)
	return offer
}

// GrantOfferAccess grants rights for an application offer.
func (j *JIMM) GrantOfferAccess(ctx context.Context, u *dbmodel.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
	const op = errors.Op("jimm.GrantOfferAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: u.Tag().String(),
		Action:  "grant",
		Params: dbmodel.StringMap{
			"url":    offerURL,
			"user":   ut.String(),
			"access": string(access),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	err := j.doApplicationOfferAdmin(ctx, u, offerURL, func(offer *dbmodel.ApplicationOffer, api API) error {
		ale.Tag = offer.Tag().String()
		targetUser := dbmodel.User{
			Username: ut.Id(),
		}
		if err := j.Database.GetUser(ctx, &targetUser); err != nil {
			return err
		}
		if err := api.GrantApplicationOfferAccess(ctx, offerURL, ut, access); err != nil {
			return err
		}
		var offerAccess dbmodel.UserApplicationOfferAccess
		for _, a := range offer.Users {
			if a.Username == targetUser.Username {
				offerAccess = a
				break
			}
		}
		offerAccess.Username = targetUser.Username
		offerAccess.ApplicationOfferID = offer.ID
		offerAccess.Access = determineAccessLevelAfterGrant(offerAccess.Access, string(access))
		if err := j.Database.UpdateUserApplicationOfferAccess(ctx, &offerAccess); err != nil {
			return errors.E(err, "cannot update database after updating controller")
		}
		return nil
	})
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
func (j *JIMM) RevokeOfferAccess(ctx context.Context, user *dbmodel.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error) {
	const op = errors.Op("jimm.RevokeOfferAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: user.Tag().String(),
		Action:  "revoke",
		Params: dbmodel.StringMap{
			"url":    offerURL,
			"user":   ut.String(),
			"access": string(access),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	err = j.doApplicationOfferAdmin(ctx, user, offerURL, func(offer *dbmodel.ApplicationOffer, api API) error {
		ale.Tag = offer.Tag().String()
		targetUser := dbmodel.User{
			Username: ut.Id(),
		}
		if err := j.Database.GetUser(ctx, &targetUser); err != nil {
			return err
		}
		if err := api.RevokeApplicationOfferAccess(ctx, offerURL, ut, access); err != nil {
			return err
		}
		var offerAccess dbmodel.UserApplicationOfferAccess
		for _, a := range offer.Users {
			if a.Username == targetUser.Username {
				offerAccess = a
				break
			}
		}
		offerAccess.Username = targetUser.Username
		offerAccess.ApplicationOfferID = offer.ID
		offerAccess.Access = determineAccessLevelAfterRevoke(offerAccess.Access, string(access))
		if err := j.Database.UpdateUserApplicationOfferAccess(ctx, &offerAccess); err != nil {
			return errors.E(err, "cannot update database after updating controller")
		}
		return nil
	})
	if err != nil {
		return fail(errors.E(op, err))
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

	err := j.doApplicationOfferAdmin(ctx, user, offerURL, func(offer *dbmodel.ApplicationOffer, api API) error {
		ale.Tag = offer.Tag().String()
		if err := api.DestroyApplicationOffer(ctx, offerURL, force); err != nil {
			return err
		}
		if err := j.Database.DeleteApplicationOffer(ctx, offer); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fail(errors.E(op, err))
	}

	ale.Success = true
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

	api, err := j.dial(ctx, controller, offer.Model.Tag().(names.ModelTag))
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
	offerFilters = append(offerFilters, db.ApplicationOfferFilterByUser(user.Username))
	offers, err := j.Database.FindApplicationOffers(ctx, offerFilters...)
	if err != nil {
		return nil, errors.E(op, err)
	}
	offerDetails := make([]jujuparams.ApplicationOfferAdminDetails, len(offers))
	for i, offer := range offers {
		accessLevel := offer.UserAccess(user)
		if user.ControllerAccess == "superuser" || offer.Model.UserAccess(user) == "admin" {
			// If the user is a controller superuser or model admin, we want to
			// fill the offer with the same details as for an offer admin.
			accessLevel = "admin"
		}
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
					db.ApplicationOfferFilterByConsumer(user.Username),
				)
			}
		}

	}
	return filters, nil
}

// ListApplicationOffers returns details of offers matching the specified filter.
func (j *JIMM) ListApplicationOffers(ctx context.Context, user *dbmodel.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
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
			f.OwnerName = user.Username
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
			Name:          k.name,
			OwnerUsername: k.ownerUsername,
		}
		offerDetails, err := j.listApplicationOffersForModel(ctx, user, &m, modelFilters[k])
		if err != nil {
			return nil, errors.E(op, err)
		}
		for _, offer := range offerDetails {
			offer.Users = filterApplicationOfferUsers(user, "admin", offer.Users)
			offers = append(offers, offer)
		}
	}
	return offers, nil
}

func (j *JIMM) listApplicationOffersForModel(ctx context.Context, u *dbmodel.User, m *dbmodel.Model, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jimm.listApplicationOffersForModel")

	if err := j.Database.GetModel(ctx, m); err != nil {
		return nil, errors.E(op, err)
	}
	if u.ControllerAccess != "superuser" && m.UserAccess(u) != "admin" {
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

// doApplicationOfferAdmin performs the given function on an applicaiton offer
// only if the given user has admin access on the model of the offer, or is a
// controller superuser. Otherwise an unauthorized error is returned.
//
// Note: The user does not need to have any access level on the offer itself.
// As long as they are model admins or controller superusers they can also
// manipulate the application offer as admins.
func (j *JIMM) doApplicationOfferAdmin(ctx context.Context, u *dbmodel.User, offerURL string, f func(offer *dbmodel.ApplicationOffer, api API) error) error {
	const op = errors.Op("jimm.doApplicationOfferAdmin")

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	if err := j.Database.GetApplicationOffer(ctx, &offer); err != nil {
		return errors.E(op, err)
	}
	if u.ControllerAccess != "superuser" && offer.UserAccess(u) != "admin" && offer.Model.UserAccess(u) != "admin" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	api, err := j.dial(ctx, &offer.Model.Controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	if err := f(&offer, api); err != nil {
		return errors.E(op, err)
	}
	return nil
}
