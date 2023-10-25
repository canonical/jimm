// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/apiconn"
	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/conv"
	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/internal/zapctx"
	"github.com/canonical/jimm/internal/zaputil"
	"github.com/canonical/jimm/params"
)

// Offer creates a new application offer.
func (j *JEM) Offer(ctx context.Context, id identchecker.ACLIdentity, offer jujuparams.AddApplicationOffer) (err error) {
	errorChannel := make(chan error, 1)
	go func() {
		ctx := context.Background()
		err := j.offer1(ctx, id, offer)
		select {
		case errorChannel <- err:
		default:
			zapctx.Warn(ctx, "failed to return the offer result", zaputil.Error(err))
		}
	}()

	select {
	case err := <-errorChannel:
		return err
	case <-ctx.Done():
		zapctx.Warn(ctx, "context cancelled")
		return ctx.Err()
	}
}

func (j *JEM) offer1(ctx context.Context, id identchecker.ACLIdentity, offer jujuparams.AddApplicationOffer) (err error) {
	modelTag, err := names.ParseModelTag(offer.ModelTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}

	model := mongodoc.Model{
		UUID: modelTag.Id(),
	}
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, &model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	conn, err := j.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	err = conn.Offer(ctx, offer)
	if err != nil {
		if !apiconn.IsAPIError(err) || !strings.Contains(err.Error(), "application offer already exists") {
			return errgo.Mask(err, apiconn.IsAPIError)
		}
	}

	offerURL := conv.ToOfferURL(model.Path, offer.OfferName)

	// Ensure the user creating the offer is an admin for the offer.
	if err := conn.GrantApplicationOfferAccess(ctx, offerURL, params.User(id.Id()), jujuparams.OfferAdminAccess); err != nil {
		return errgo.Mask(err)
	}

	offerDetails := jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			OfferURL: offerURL,
		},
	}
	err = conn.GetApplicationOffer(ctx, &offerDetails)
	if err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}

	doc := offerDetailsToMongodoc(ctx, &model, offerDetails)

	err = j.DB.InsertApplicationOffer(ctx, &doc)
	if err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return nil
		}
		return errgo.Mask(err)
	}

	return nil
}

// GetApplicationOfferConsumeDetails consume the application offer
// specified by details.ApplicationOfferDetails.OfferURL and completes
// the rest of the details.
func (j *JEM) GetApplicationOfferConsumeDetails(ctx context.Context, id identchecker.ACLIdentity, user params.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	offer := mongodoc.ApplicationOffer{
		OfferURL: details.Offer.OfferURL,
	}
	if err := j.DB.GetApplicationOffer(ctx, &offer); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	uid := params.User(strings.ToLower(id.Id()))
	access := offer.Users[mongodoc.User(uid)]
	if access < mongodoc.ApplicationOfferConsumeAccess {
		// If the current user doesn't have access then check if it is
		// publicly available.
		access = offer.Users[identchecker.Everyone]
	}
	switch access {
	case mongodoc.ApplicationOfferNoAccess:
		// If the user can't even read the application offer say it doesn't
		// exist.
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	case mongodoc.ApplicationOfferReadAccess:
		return errgo.WithCausef(nil, params.ErrUnauthorized, "")
	default:
	}
	// The user has consume access or higher.
	ctl := mongodoc.Controller{
		Path: offer.ControllerPath,
	}
	if err := j.DB.GetController(ctx, &ctl); err != nil {
		return errgo.Mask(err)
	}
	conn, err := j.OpenAPIFromDoc(ctx, &ctl)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	asUser := uid
	if user != "" {
		if err := auth.CheckACL(ctx, id, j.ControllerAdmins()); err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
		asUser = user
	}
	if err := conn.GetApplicationOfferConsumeDetails(ctx, asUser, details, v); err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}

	// Fix the consume details from the controller to be correct for JAAS.
	// Filter out any juju local users.
	details.Offer.Users = filterApplicationOfferUsers(uid, access, details.Offer.Users)

	// Fix the addresses to be a controller's external addresses.
	details.ControllerInfo = &jujuparams.ExternalControllerInfo{
		ControllerTag: names.NewControllerTag(ctl.UUID).String(),
		Alias:         string(ctl.Path.Name),
		CACert:        ctl.CACert,
	}
	for _, hps := range ctl.HostPorts {
		for _, hp := range hps {
			switch hp.Scope {
			case "", "public":
				details.ControllerInfo.Addrs = append(details.ControllerInfo.Addrs, hp.Address())
			default:
				continue
			}
		}
	}

	return nil
}

// filterApplicationOfferUsers filters the application offer user list
// to be suitable for the given user at the given access level. All juju-
// local users are omitted, and if the user is not an admin then they can
// only see themselves.
func filterApplicationOfferUsers(id params.User, a mongodoc.ApplicationOfferAccessPermission, users []jujuparams.OfferUserDetails) []jujuparams.OfferUserDetails {
	filtered := make([]jujuparams.OfferUserDetails, 0, len(users))
	for _, u := range users {
		// If FromUserID returns an error then the user is a juju-local
		// user, those are skipped.
		if uid, err := conv.FromUserID(u.UserName); err == nil {
			if a == mongodoc.ApplicationOfferAdminAccess || uid == id || uid == identchecker.Everyone {
				filtered = append(filtered, u)
			}
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UserName < filtered[j].UserName
	})
	return filtered
}

// ListApplicationOffers returns details of offers matching the specified filter.
func (j *JEM) ListApplicationOffers(ctx context.Context, id identchecker.ACLIdentity, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	uid := params.User(strings.ToLower(id.Id()))

	q := applicationOffersQuery(uid, mongodoc.ApplicationOfferAdminAccess, filters)
	controllerOffers := make(map[params.EntityPath][]mongodoc.ApplicationOffer)
	err := j.DB.ForEachApplicationOffer(ctx, q, []string{"owner-name", "model-name", "offer-name"}, func(offer *mongodoc.ApplicationOffer) error {
		offers := controllerOffers[offer.ControllerPath]
		offers = append(offers, *offer)
		controllerOffers[offer.ControllerPath] = offers
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	var offerDetails []jujuparams.ApplicationOfferAdminDetails
	for controller, offers := range controllerOffers {
		details, err := j.getApplicationOfferDetailsFromController(ctx, uid, controller, offers)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		offerDetails = append(offerDetails, details...)
	}

	return offerDetails, nil
}

// FindApplicationOffers returns details of offers matching the specified filter.
func (j *JEM) FindApplicationOffers(ctx context.Context, id identchecker.ACLIdentity, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	uid := params.User(strings.ToLower(id.Id()))

	if len(filters) == 0 {
		return nil, errgo.WithCausef(nil, params.ErrBadRequest, "at least one filter must be specified")
	}

	q := applicationOffersQuery(uid, mongodoc.ApplicationOfferReadAccess, filters)
	controllerOffers := make(map[params.EntityPath][]mongodoc.ApplicationOffer)
	err := j.DB.ForEachApplicationOffer(ctx, q, []string{"owner-name", "model-name", "offer-name"}, func(offer *mongodoc.ApplicationOffer) error {
		offers := controllerOffers[offer.ControllerPath]
		offers = append(offers, *offer)
		controllerOffers[offer.ControllerPath] = offers
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	var offerDetails []jujuparams.ApplicationOfferAdminDetails
	for controller, offers := range controllerOffers {
		details, err := j.getApplicationOfferDetailsFromController(ctx, uid, controller, offers)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		offerDetails = append(offerDetails, details...)
	}
	return offerDetails, errgo.Mask(err)
}

func applicationOffersQuery(u params.User, access mongodoc.ApplicationOfferAccessPermission, filters []jujuparams.OfferFilter) jimmdb.Query {
	userQuery := jimmdb.Or(jimmdb.Gte(mongodoc.User(u).FieldName("users"), access), jimmdb.Gte("users.everyone", access))
	fqs := make([]jimmdb.Query, 0, len(filters))
	for _, f := range filters {
		q := filterQuery(f)
		if q == nil {
			continue
		}
		fqs = append(fqs, q)
	}

	if len(fqs) > 0 {
		return jimmdb.And(userQuery, jimmdb.Or(fqs...))
	}
	return userQuery
}

func filterQuery(f jujuparams.OfferFilter) jimmdb.Query {
	qs := make([]jimmdb.Query, 0, 7)
	if f.OwnerName != "" {
		qs = append(qs, jimmdb.Eq("owner-name", f.OwnerName))
	}

	if f.ModelName != "" {
		qs = append(qs, jimmdb.Eq("model-name", f.ModelName))
	}

	if f.ApplicationName != "" {
		qs = append(qs, jimmdb.Eq("application-name", f.ApplicationName))
	}

	// We match on partial names, eg "-sql"
	if f.OfferName != "" {
		qs = append(qs, jimmdb.Regex("offer-name", fmt.Sprintf(".*%s.*", f.OfferName)))
	}

	// We match descriptions by looking for containing terms.
	if f.ApplicationDescription != "" {
		desc := regexp.QuoteMeta(f.ApplicationDescription)
		qs = append(qs, jimmdb.Regex("application-description", fmt.Sprintf(".*%s.*", desc)))
	}

	if len(f.Endpoints) > 0 {
		endpoints := make([]jimmdb.Query, 0, len(f.Endpoints))
		for _, ep := range f.Endpoints {
			match := make([]jimmdb.Query, 0, 3)
			if ep.Interface != "" {
				match = append(match, jimmdb.Eq("interface", ep.Interface))
			}
			if ep.Name != "" {
				match = append(match, jimmdb.Eq("name", ep.Name))
			}
			if ep.Role != "" {
				match = append(match, jimmdb.Eq("role", ep.Role))
			}
			if len(match) == 0 {
				continue
			}
			endpoints = append(endpoints, jimmdb.ElemMatch("endpoints", jimmdb.And(match...)))
		}
		if len(endpoints) > 0 {
			qs = append(qs, jimmdb.Or(endpoints...))
		}
	}

	if len(f.AllowedConsumerTags) > 0 {
		users := make([]jimmdb.Query, 0, len(f.AllowedConsumerTags))
		for _, userTag := range f.AllowedConsumerTags {
			user, err := conv.ParseUserTag(userTag)
			if err != nil {
				// If this user does not parse then it will never match
				// a record, add a query that can't match.
				users = append(users, jimmdb.Eq("users.~~impossible", "impossible"))
				continue
			}

			users = append(users, jimmdb.Gte(mongodoc.User(user).FieldName("users"), mongodoc.ApplicationOfferConsumeAccess))
		}
		switch len(users) {
		case 1:
			qs = append(qs, users[0])
		default:
			qs = append(qs, jimmdb.Or(users...))
		case 0:
		}
	}

	switch len(qs) {
	case 0:
		return nil
	case 1:
		return qs[0]
	default:
		return jimmdb.And(qs...)
	}
}

// GetApplicationOffer returns details of the offer with the specified URL.
func (j *JEM) GetApplicationOffer(ctx context.Context, id identchecker.ACLIdentity, offerURL string) (*jujuparams.ApplicationOfferAdminDetails, error) {
	uid := params.User(strings.ToLower(id.Id()))

	offer := mongodoc.ApplicationOffer{
		OfferURL: offerURL,
	}
	err := j.DB.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	access := offer.Users[mongodoc.User(uid)]
	if access == mongodoc.ApplicationOfferNoAccess {
		access = offer.Users[identchecker.Everyone]
	}
	// one needs at least read access to get the application offer
	if access < mongodoc.ApplicationOfferReadAccess {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	offerDetails, err := j.getApplicationOfferDetails(ctx, uid, &offer)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &offerDetails, nil
}

func (j *JEM) getApplicationOfferDetailsFromController(ctx context.Context, uid params.User, controllerPath params.EntityPath, offers []mongodoc.ApplicationOffer) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	conn, err := j.OpenAPI(ctx, controllerPath)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	defer conn.Close()

	offerDetails := make([]*jujuparams.ApplicationOfferAdminDetails, len(offers))
	for i, offer := range offers {
		details := jujuparams.ApplicationOfferAdminDetails{
			ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
				OfferURL: offer.OfferURL,
			},
		}
		offerDetails[i] = &details
	}

	if err := conn.GetApplicationOffers(ctx, offerDetails); err != nil {
		return nil, errgo.Mask(err)
	}

	results := make([]jujuparams.ApplicationOfferAdminDetails, len(offerDetails))
	for i, details := range offerDetails {
		details := *details

		access := offers[i].Users[mongodoc.User(uid)]
		if access == mongodoc.ApplicationOfferNoAccess {
			access = offers[i].Users[identchecker.Everyone]
		}

		details.Users = filterApplicationOfferUsers(uid, access, details.Users)
		if access != mongodoc.ApplicationOfferAdminAccess {
			details.Connections = nil
		}
		results[i] = details
	}

	return results, nil
}

func (j *JEM) getApplicationOfferDetails(ctx context.Context, uid params.User, offer *mongodoc.ApplicationOffer) (jujuparams.ApplicationOfferAdminDetails, error) {
	access := offer.Users[mongodoc.User(uid)]
	if access == mongodoc.ApplicationOfferNoAccess {
		access = offer.Users[identchecker.Everyone]
	}

	conn, err := j.OpenAPI(ctx, offer.ControllerPath)
	if err != nil {
		return jujuparams.ApplicationOfferAdminDetails{}, errgo.Mask(err)
	}
	defer conn.Close()
	var offerDetails jujuparams.ApplicationOfferAdminDetails
	offerDetails.OfferURL = offer.OfferURL

	if err := conn.GetApplicationOffer(ctx, &offerDetails); err != nil {
		return jujuparams.ApplicationOfferAdminDetails{}, errgo.Mask(err)
	}

	offerDetails.Users = filterApplicationOfferUsers(uid, access, offerDetails.Users)
	if access != mongodoc.ApplicationOfferAdminAccess {
		offerDetails.Connections = nil
	}

	return offerDetails, nil
}

// GrantOfferAccess grants rights for an application offer.
func (j *JEM) GrantOfferAccess(ctx context.Context, id identchecker.ACLIdentity, user params.User, offerURL string, access jujuparams.OfferAccessPermission) (err error) {
	uid := params.User(strings.ToLower(id.Id()))
	user = params.User(strings.ToLower(string(user)))

	// first we need to fetch the offer to get it's UUID
	var offer mongodoc.ApplicationOffer
	offer.OfferURL = offerURL
	err = j.DB.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// retrieve the access rights for the authenticated user
	offerAccess := offer.Users[mongodoc.User(uid)]

	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch offerAccess {
	case mongodoc.ApplicationOfferNoAccess:
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	case mongodoc.ApplicationOfferReadAccess, mongodoc.ApplicationOfferConsumeAccess:
		return errgo.WithCausef(nil, params.ErrUnauthorized, "")
	default:
	}

	var permission mongodoc.ApplicationOfferAccessPermission
	switch access {
	case jujuparams.OfferAdminAccess:
		permission = mongodoc.ApplicationOfferAdminAccess
	case jujuparams.OfferConsumeAccess:
		permission = mongodoc.ApplicationOfferConsumeAccess
	case jujuparams.OfferReadAccess:
		permission = mongodoc.ApplicationOfferReadAccess
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "unknown permission level")
	}

	// grant access on the controller
	conn, err := j.OpenAPI(ctx, offer.ControllerPath)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	err = conn.GrantApplicationOfferAccess(ctx, offer.OfferURL, user, access)
	if err != nil {
		return errgo.Mask(err)
	}

	// then grant access in the jimm DB
	u := new(jimmdb.Update).Set(mongodoc.User(user).FieldName("users"), permission)
	if err := j.DB.UpdateApplicationOffer(ctx, &offer, u, true); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// RevokeOfferAccess revokes rights for an application offer.
func (j *JEM) RevokeOfferAccess(ctx context.Context, id identchecker.ACLIdentity, user params.User, offerURL string, access jujuparams.OfferAccessPermission) (err error) {
	uid := params.User(strings.ToLower(id.Id()))

	// first we need to fetch the offer to get it's UUID
	var offer mongodoc.ApplicationOffer
	offer.OfferURL = offerURL
	err = j.DB.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// retrieve the access rights for the authenticated user
	offerAccess := offer.Users[mongodoc.User(uid)]

	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch offerAccess {
	case mongodoc.ApplicationOfferNoAccess:
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	case mongodoc.ApplicationOfferReadAccess, mongodoc.ApplicationOfferConsumeAccess:
		return errgo.WithCausef(nil, params.ErrUnauthorized, "")
	default:
	}

	// revoking access level L results in user
	// retainig access level L-1
	var permission mongodoc.ApplicationOfferAccessPermission
	switch access {
	case jujuparams.OfferAdminAccess:
		permission = mongodoc.ApplicationOfferConsumeAccess
	case jujuparams.OfferConsumeAccess:
		permission = mongodoc.ApplicationOfferReadAccess
	case jujuparams.OfferReadAccess:
		permission = mongodoc.ApplicationOfferNoAccess
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "unknown permission level")
	}

	// revoke on the actual controller - should this fail
	// the record of access remains in jimm and we can retry
	// later
	conn, err := j.OpenAPI(ctx, offer.ControllerPath)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	err = conn.RevokeApplicationOfferAccess(ctx, offer.OfferURL, user, access)
	if err != nil {
		return errgo.Mask(err)
	}

	// then revoke access in the jimm DB
	u := new(jimmdb.Update).Set(mongodoc.User(user).FieldName("users"), permission)
	if err := j.DB.UpdateApplicationOffer(ctx, &offer, u, true); err != nil {
		return errgo.Mask(err)
	}

	return nil
}

// DestroyOffer removes the application offer.
func (j *JEM) DestroyOffer(ctx context.Context, id identchecker.ACLIdentity, offerURL string, force bool) (err error) {
	uid := params.User(strings.ToLower(id.Id()))

	// first we need to fetch the offer to get it's UUID
	var offer mongodoc.ApplicationOffer
	offer.OfferURL = offerURL
	err = j.DB.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// retrieve the access rights for the authenticated user
	offerAccess := offer.Users[mongodoc.User(uid)]

	// if the authenticated user has no access to the offer, we
	// return a not found error.
	// if the user has consume or read access we return an unauthorized error.
	switch offerAccess {
	case mongodoc.ApplicationOfferNoAccess:
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	case mongodoc.ApplicationOfferReadAccess, mongodoc.ApplicationOfferConsumeAccess:
		return errgo.WithCausef(nil, params.ErrUnauthorized, "")
	default:
	}

	// first remove from the actual controller - should
	// this fail, we can always retry because a record will
	// still exist in jimm
	conn, err := j.OpenAPI(ctx, offer.ControllerPath)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	err = conn.DestroyApplicationOffer(ctx, offerURL, force)
	if err != nil {
		zapctx.Error(ctx,
			"failed to destroy application offer",
			zap.String("controller", offer.ControllerPath.String()),
			zap.String("offer", offer.OfferURL),
		)
		return errgo.Mask(err)
	}

	// then remove application offer from the jimm DB
	err = j.DB.RemoveApplicationOffer(ctx, &offer)
	if err != nil {
		return errgo.Mask(err)
	}

	return nil
}

// UpdateApplicationOffer fetches offer details from the controller and updates the
// application offer in JIMM DB.
func (j *JEM) UpdateApplicationOffer(ctx context.Context, ctlPath params.EntityPath, offerUUID string, removed bool) error {
	offer := mongodoc.ApplicationOffer{
		OfferUUID:      offerUUID,
		ControllerPath: ctlPath,
	}

	if removed {
		return errgo.Mask(j.DB.RemoveApplicationOffer(ctx, &offer))
	}

	if err := j.DB.GetApplicationOffer(ctx, &offer); err != nil {
		return errgo.Mask(err)
	}

	// Get the updated offer from the controller.
	conn, err := j.OpenAPI(ctx, ctlPath)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	offerDetails := jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			OfferURL: offer.OfferURL,
		},
	}
	err = conn.GetApplicationOffer(ctx, &offerDetails)
	if err != nil {
		return errgo.Mask(err)
	}

	u := new(jimmdb.Update)
	u.Set("application-description", offerDetails.ApplicationDescription)
	u.Set("application-name", offerDetails.ApplicationName)
	u.Set("bindings", offerDetails.Bindings)
	u.Set("charm-url", offerDetails.CharmURL)
	u.Set("connections", offerConnectionsToMongodoc(ctx, offerDetails.Connections))
	u.Set("endpoints", offerEndpointsToMongodoc(ctx, offerDetails.Endpoints))
	u.Set("offer-name", offerDetails.OfferName)
	u.Set("offer-url", offerDetails.OfferURL)

	return errgo.Mask(j.DB.UpdateApplicationOffer(ctx, &offer, u, false))
}

func offerDetailsToMongodoc(ctx context.Context, model *mongodoc.Model, offerDetails jujuparams.ApplicationOfferAdminDetails) mongodoc.ApplicationOffer {
	return mongodoc.ApplicationOffer{
		ModelUUID:              model.UUID,
		ModelName:              string(model.Path.Name),
		ControllerPath:         model.Controller,
		OfferUUID:              offerDetails.OfferUUID,
		OfferURL:               offerDetails.OfferURL,
		OfferName:              offerDetails.OfferName,
		OwnerName:              conv.ToUserTag(model.Path.User).Id(),
		ApplicationName:        offerDetails.ApplicationName,
		ApplicationDescription: offerDetails.ApplicationDescription,
		Endpoints:              offerEndpointsToMongodoc(ctx, offerDetails.Endpoints),
		Spaces:                 offerSpacesToMongodoc(ctx, offerDetails.Spaces),
		Bindings:               offerDetails.Bindings,
		Users:                  offerUsersToMongodoc(ctx, offerDetails.Users),
		Connections:            offerConnectionsToMongodoc(ctx, offerDetails.Connections),
	}
}

func offerConnectionsToMongodoc(ctx context.Context, connections []jujuparams.OfferConnection) []mongodoc.OfferConnection {
	conns := make([]mongodoc.OfferConnection, len(connections))
	for i, connection := range connections {
		conns[i] = mongodoc.OfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationId:     connection.RelationId,
			Username:       connection.Username,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
			Status:         offerConnectionStatusToMongodoc(ctx, connection.Status),
		}
	}
	return conns
}

func offerConnectionStatusToMongodoc(ctx context.Context, status jujuparams.EntityStatus) mongodoc.OfferConnectionStatus {
	return mongodoc.OfferConnectionStatus{
		Status: status.Status.String(),
		Info:   status.Info,
		Data:   status.Data,
		Since:  status.Since,
	}
}

func offerEndpointsToMongodoc(ctx context.Context, endpoints []jujuparams.RemoteEndpoint) []mongodoc.RemoteEndpoint {
	eps := make([]mongodoc.RemoteEndpoint, len(endpoints))
	for i, endpoint := range endpoints {
		eps[i] = mongodoc.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      string(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	return eps
}

func offerUsersToMongodoc(ctx context.Context, users []jujuparams.OfferUserDetails) map[mongodoc.User]mongodoc.ApplicationOfferAccessPermission {
	accesses := make(map[mongodoc.User]mongodoc.ApplicationOfferAccessPermission, len(users))
	for _, user := range users {
		pu, err := conv.FromUserID(strings.ToLower(user.UserName))
		if err != nil {
			// If we can't parse the user, it's either a local user which
			// we don't store, or an invalid user which can't do anything.
			zapctx.Warn(ctx, "cannot parse username", zap.String("username", user.UserName))
			continue
		}
		var access mongodoc.ApplicationOfferAccessPermission
		switch user.Access {
		case string(jujuparams.OfferAdminAccess):
			access = mongodoc.ApplicationOfferAdminAccess
		case string(jujuparams.OfferConsumeAccess):
			access = mongodoc.ApplicationOfferConsumeAccess
		case string(jujuparams.OfferReadAccess):
			access = mongodoc.ApplicationOfferReadAccess
		default:
			zapctx.Warn(ctx, "unknown access level", zap.String("level", user.Access))
			continue
		}
		accesses[mongodoc.User(pu)] = access
	}
	return accesses
}

func offerSpacesToMongodoc(ctx context.Context, spaces []jujuparams.RemoteSpace) []mongodoc.RemoteSpace {
	ss := make([]mongodoc.RemoteSpace, len(spaces))
	for i, space := range spaces {
		ss[i] = mongodoc.RemoteSpace{
			CloudType:          space.CloudType,
			Name:               space.Name,
			ProviderId:         space.ProviderId,
			ProviderAttributes: space.ProviderAttributes,
		}
	}
	return ss
}
