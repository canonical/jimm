// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/core/crossmodel"
	jujupermission "github.com/juju/juju/core/permission"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
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
func (j *JujuManager) Offer(ctx context.Context, user *openfga.User, offer AddApplicationOfferParams) error {

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: offer.ModelTag.Id(),
			Valid:  true,
		},
	}
	if err := j.Database.GetModel(ctx, &model); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(err, "model not found")
		}
		return errors.E(err)
	}

	isAdmin, err := openfga.IsAdministrator(ctx, user, model.ResourceTag())
	if err != nil {
		return errors.E(fmt.Errorf("failed administrator check: %w", err))
	}
	if !isAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	offerURL := crossmodel.OfferURL{
		User:      model.OwnerIdentityName,
		ModelName: model.Name,
		// Confusingly the application name in the offer URL is
		// actually the offer name.
		ApplicationName: offer.OfferName,
	}

	// Verify offer URL doesn't already exist.
	var offerCheck dbmodel.ApplicationOffer
	offerCheck.URL = offerURL.String()
	err = j.Database.GetApplicationOffer(ctx, &offerCheck)
	if err == nil {
		return errors.E(fmt.Sprintf("offer %s already exists, please use a different name", offerURL.String()), errors.CodeAlreadyExists)
	} else if errors.ErrorCode(err) != errors.CodeNotFound {
		// Anything besides Not Found is a problem.
		return errors.E(err)
	}

	api, err := j.dial(ctx, &model.Controller, names.ModelTag{}, nil)
	if err != nil {
		return errors.E(err)
	}
	defer api.Close()

	owner := offer.OwnerTag.Id()
	if owner == "" {
		owner = user.Tag().Id()
	}
	var endpoints []string
	for name := range offer.Endpoints {
		endpoints = append(endpoints, name)
	}
	err = api.Offer(ctx,
		jujuclient.OfferParams{
			ModelUUID:   offer.ModelTag.Id(),
			Owner:       owner,
			OfferName:   offer.OfferName,
			Application: offer.ApplicationName,
			Desc:        offer.ApplicationDescription,
			Endpoints:   endpoints,
		})
	if err != nil {
		if strings.Contains(err.Error(), "application offer already exists") {
			return errors.E(err, errors.CodeAlreadyExists)
		}
		return errors.E(err)
	}

	createdAppOffer, err := api.GetApplicationOffer(ctx, offerURL.String())
	if err != nil {
		return errors.E(fmt.Errorf("failed to fetch details of the created application offer: %w", err))
	}

	doc := dbmodel.ApplicationOffer{
		ModelID: model.ID,
		Name:    createdAppOffer.OfferName,
		UUID:    createdAppOffer.OfferUUID,
		URL:     createdAppOffer.OfferURL,
	}
	err = j.Database.Transaction(func(db *db.Database) error {
		if err := db.AddApplicationOffer(ctx, &doc); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return errors.E(fmt.Errorf("failed to store the created application offer: %w", err))
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
		return errors.E(err)
	}

	ownerUser := openfga.NewUser(
		identity,
		j.OpenFGAClient,
	)
	if err := ownerUser.SetApplicationOfferAccess(ctx, doc.ResourceTag(), ofganames.AdministratorRelation); err != nil {
		zapctx.Error(
			ctx,
			"failed relation between user and application offer",
			zap.String("user", ownerId),
			zap.String("application-offer", doc.UUID))
	}

	if err := j.everyoneUser().SetApplicationOfferAccess(ctx, doc.ResourceTag(), ofganames.ReaderRelation); err != nil {
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
func (j *JujuManager) GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {

	offer := dbmodel.ApplicationOffer{
		URL: details.Offer.OfferURL,
	}
	if err := j.Database.GetApplicationOffer(ctx, &offer); err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(err, "application offer not found")
		}
		return errors.E(err)
	}

	accessLevel, err := j.getUserOfferAccess(ctx, user, offer.ResourceTag())
	if err != nil {
		return errors.E(err)
	}

	switch accessLevel {
	case string(jujuparams.OfferAdminAccess):
	case string(jujuparams.OfferConsumeAccess):
	case string(jujuparams.OfferReadAccess):
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	default:
		// TODO (ashipika)
		//   - think about the returned error code
		return errors.E(errors.CodeNotFound)
	}

	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		user,
		permission{
			resource: names.NewApplicationOfferTag(offer.UUID).String(),
			relation: accessLevel,
		},
	)
	if err != nil {
		return errors.E(err)
	}
	defer api.Close()

	consumeDetails, err := api.GetApplicationOfferConsumeDetails(ctx, details.Offer.OfferURL)
	if err != nil {
		return errors.E(err)
	}
	details.Offer = consumeDetails.Offer
	details.ControllerInfo = consumeDetails.ControllerInfo
	details.Macaroon = consumeDetails.Macaroon

	// Fix the consume details from the controller to be correct for JAAS.
	// Filter out any juju local users.
	users, err := j.listApplicationOfferUsers(ctx, offer.ResourceTag(), user.Identity, accessLevel == string(jujuparams.OfferAdminAccess))
	if err != nil {
		return errors.E(err)
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
func (j *JujuManager) listApplicationOfferUsers(ctx context.Context, offer names.ApplicationOfferTag, user *dbmodel.Identity, adminAccess bool) ([]jujuparams.OfferUserDetails, error) {
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
			users[user.Name] = permissions.ToOfferAccessString(relation)
		}
	}

	userDetails := []jujuparams.OfferUserDetails{}
	for username, level := range users {
		// non-admin users should only see their own access level
		// and the access level of "everyone" - meaning the access
		// level everybody has.
		if !adminAccess && username != ofganames.EveryoneUser && username != user.Name {
			continue
		}
		userDetails = append(userDetails, jujuparams.OfferUserDetails{
			UserName: username,
			Access:   level,
		})
	}
	return userDetails, nil
}

var noApplicationOfferAccessError = errors.E("no application offer access")

// enrichOfferDetails replaces fields on an application offer's details with information
// where JIMM is authoritative. It returns a noApplicationOfferAccessError if the user
// does not have access to the offer.
func (j *JujuManager) enrichOfferDetails(ctx context.Context, user *openfga.User, offerDetail *crossmodel.ApplicationOfferDetails) error {
	if offerDetail == nil {
		return errors.E("offerDetail cannot be nil")
	}
	// TODO (alesstimec) Optimize this: currently check all possible
	// permission levels for an offer, this is suboptimal.
	if !names.IsValidApplicationOffer(offerDetail.OfferUUID) {
		return errors.E("invalid application offer UUID")
	}
	offerTag := names.NewApplicationOfferTag(offerDetail.OfferUUID)
	accessLevel, err := j.getUserOfferAccess(ctx, user, offerTag)
	if err != nil {
		return err
	}

	if accessLevel == "" {
		return noApplicationOfferAccessError
	}

	// non-admin users should not see connections of an application
	// offer.
	if accessLevel != "admin" {
		offerDetail.Connections = nil
	}
	users, err := j.listApplicationOfferUsers(ctx, offerTag, user.Identity, accessLevel == "admin")
	if err != nil {
		return err
	}

	var offerUsers []crossmodel.OfferUserDetails
	for _, user := range users {
		offerUsers = append(offerUsers, crossmodel.OfferUserDetails{
			UserName:    user.UserName,
			Access:      jujupermission.Access(user.Access),
			DisplayName: user.DisplayName,
		})
	}
	offerDetail.Users = offerUsers

	return nil
}

// GetApplicationOffer returns details of the offer with the specified URL.
func (j *JujuManager) GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*crossmodel.ApplicationOfferDetails, error) {

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err := j.Database.GetApplicationOffer(ctx, &offer)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, errors.E(err, "application offer not found")
		}
		return nil, errors.E(err)
	}

	reader, err := user.IsApplicationOfferReader(ctx, offer.ResourceTag())
	if err != nil {
		return nil, errors.E(err)
	}

	// if this user does not have access to this application offer
	// we return a not found error.
	if !reader {
		return nil, errors.E(errors.CodeNotFound, "application offer not found")
	}

	// Always collect application-offer admin details from the
	// controller. The all-watcher events do not include enough
	// information to reasonably keep the local database up-to-date,
	// and it would be non-trivial to make it do so.
	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		nil,
	)
	if err != nil {
		return nil, errors.E(err)
	}
	defer api.Close()

	offerDetails, err := api.GetApplicationOffer(ctx, offerURL)
	if err != nil {
		return nil, errors.E(err)
	}

	err = j.enrichOfferDetails(ctx, user, offerDetails)
	if err != nil {
		return nil, errors.E(err)
	}

	return offerDetails, nil
}

// DestroyOffer removes the application offer.
func (j *JujuManager) DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error {

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
		return errors.E(err)
	}

	return nil
}

// getUserOfferAccess returns the access level string for the user to the
// application offer. It returns the highest access level the user is granted.
func (j *JujuManager) getUserOfferAccess(ctx context.Context, user *openfga.User, offerTag names.ApplicationOfferTag) (string, error) {
	isOfferAdmin, err := openfga.IsAdministrator(ctx, user, offerTag)
	if err != nil {
		return "", errors.E(fmt.Errorf("openfga check failed: %w", err))
	}
	if isOfferAdmin {
		return string(jujuparams.OfferAdminAccess), nil
	}
	isOfferConsumer, err := user.IsApplicationOfferConsumer(ctx, offerTag)
	if err != nil {
		return "", errors.E(fmt.Errorf("openfga check failed: %w", err))
	}
	if isOfferConsumer {
		return string(jujuparams.OfferConsumeAccess), nil
	}
	isOfferReader, err := user.IsApplicationOfferReader(ctx, offerTag)
	if err != nil {
		return "", errors.E(fmt.Errorf("openfga check failed: %w", err))
	}
	if isOfferReader {
		return string(jujuparams.OfferReadAccess), nil
	}
	return "", nil
}

type offers struct {
	mu     sync.Mutex
	offers []*crossmodel.ApplicationOfferDetails
}

func (o *offers) addOffer(offer *crossmodel.ApplicationOfferDetails) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.offers = append(o.offers, offer)
}

// FindApplicationOffers returns details of offers matching the specified filter.
func (j *JujuManager) FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {

	if len(filters) == 0 {
		return nil, errors.E(errors.CodeBadRequest, "at least one filter must be specified")
	}

	controllers := make(map[uint]*dbmodel.Controller)
	err := j.Database.ForEachController(ctx, func(ctl *dbmodel.Controller) error {
		controllers[ctl.ID] = ctl
		return nil
	})
	if err != nil {
		return nil, errors.E(err)
	}

	offers, err := j.queryControllersForOffers(ctx, user, controllers, func(api API) ([]*crossmodel.ApplicationOfferDetails, error) {
		return api.FindApplicationOffers(ctx, filters)
	})
	if err != nil {
		return nil, errors.E(err)
	}
	return offers, nil
}

// ListApplicationOffers returns details of offers matching the specified filter.
func (j *JujuManager) ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {

	if len(filters) == 0 {
		return nil, errors.E(errors.CodeBadRequest, "at least one filter must be specified")
	}

	controllers := make(map[uint]*dbmodel.Controller)
	for _, f := range filters {
		if f.ModelName == "" {
			return nil, errors.E("application offer filter must specify a model name")
		}
		if f.OwnerName == "" {
			f.OwnerName = user.Name
		}

		m := dbmodel.Model{
			Name:              f.ModelName,
			OwnerIdentityName: f.OwnerName,
		}
		if err := j.Database.GetModel(ctx, &m); err != nil {
			return nil, errors.E(err)
		}
		controllers[m.Controller.ID] = &m.Controller
	}

	offers, err := j.queryControllersForOffers(ctx, user, controllers, func(api API) ([]*crossmodel.ApplicationOfferDetails, error) {
		return api.ListApplicationOffers(ctx, filters)
	})
	if err != nil {
		return nil, errors.E(err)
	}
	return offers, nil
}

func (j *JujuManager) queryControllersForOffers(ctx context.Context, user *openfga.User, controllers map[uint]*dbmodel.Controller, query func(API) ([]*crossmodel.ApplicationOfferDetails, error)) ([]*crossmodel.ApplicationOfferDetails, error) {
	var offerDetails offers
	eg, ctx := errgroup.WithContext(ctx)

	for _, ctl := range controllers {
		eg.Go(func() error {
			// Return early if a single controller has an error
			// to avoid misleading clients about what exists which
			// could cause unneeded reconciliation.
			api, err := j.dial(ctx, ctl, names.ModelTag{}, nil)
			if err != nil {
				return errors.E(err)
			}
			defer api.Close()
			controllerOffers, err := query(api)
			if err != nil {
				if errors.ErrorCode(err) == errors.CodeNotFound {
					return nil
				}
				return errors.E(err)
			}
			for _, offer := range controllerOffers {
				err = j.enrichOfferDetails(ctx, user, offer)
				if err != nil {
					if stderrors.Is(err, noApplicationOfferAccessError) {
						continue
					}
					return errors.E(err)
				}

				offerDetails.addOffer(offer)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return offerDetails.offers, err
	}

	return offerDetails.offers, nil
}

// doApplicationOfferAdmin performs the given function on an application offer
// only if the given user has admin access on the model of the offer, or is a
// controller superuser. Otherwise an unauthorized error is returned.
//
// Note: The user does not need to have any access level on the offer itself.
// As long as they are model admins or controller superusers they can also
// manipulate the application offer as admins.
func (j *JujuManager) doApplicationOfferAdmin(ctx context.Context, user *openfga.User, offerURL string, f func(offer *dbmodel.ApplicationOffer, api API) error) error {

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	if err := j.Database.GetApplicationOffer(ctx, &offer); err != nil {
		return errors.E(err)
	}

	isOfferAdmin, err := openfga.IsAdministrator(ctx, user, offer.ResourceTag())
	if err != nil {
		return errors.E(err)
	}
	if !isOfferAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	// add offer admin claim
	api, err := j.dial(
		ctx,
		&offer.Model.Controller,
		names.ModelTag{},
		nil,
	)
	if err != nil {
		return errors.E(err)
	}
	defer api.Close()
	if err := f(&offer, api); err != nil {
		return errors.E(err)
	}
	return nil
}
