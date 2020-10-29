// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/charm/v7"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

// Offer creates a new application offer.
func (j *JEM) Offer(ctx context.Context, id identchecker.ACLIdentity, offer jujuparams.AddApplicationOffer) (err error) {
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

	doc := offerDetailsToMongodoc(&model, offerDetails)

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
func (j *JEM) GetApplicationOfferConsumeDetails(ctx context.Context, id identchecker.ACLIdentity, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	offer := mongodoc.ApplicationOffer{
		OfferURL: details.Offer.OfferURL,
	}
	if err := j.DB.GetApplicationOffer(ctx, &offer); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	uid := params.User(id.Id())
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

	if err := conn.GetApplicationOfferConsumeDetails(ctx, details, v); err != nil {
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
			if a == mongodoc.ApplicationOfferAdminAccess || uid == id {
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
	uid := params.User(id.Id())

	q := applicationOffersQuery(uid, mongodoc.ApplicationOfferAdminAccess, filters)
	var offers []jujuparams.ApplicationOfferAdminDetails
	err := j.DB.ForEachApplicationOffer(ctx, q, []string{"owner-name", "model-name", "offer-name"}, func(offer *mongodoc.ApplicationOffer) error {
		offers = append(offers, applicationOfferDocToDetails(uid, offer))
		return nil
	})
	return offers, errgo.Mask(err)
}

// FindApplicationOffers returns details of offers matching the specified filter.
func (j *JEM) FindApplicationOffers(ctx context.Context, id identchecker.ACLIdentity, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	uid := params.User(id.Id())

	if len(filters) == 0 {
		return nil, errgo.WithCausef(nil, params.ErrBadRequest, "at least one filter must be specified")
	}

	q := applicationOffersQuery(uid, mongodoc.ApplicationOfferReadAccess, filters)
	var offers []jujuparams.ApplicationOfferAdminDetails
	err := j.DB.ForEachApplicationOffer(ctx, q, []string{"owner-name", "model-name", "offer-name"}, func(offer *mongodoc.ApplicationOffer) error {
		offers = append(offers, applicationOfferDocToDetails(uid, offer))
		return nil
	})
	return offers, errgo.Mask(err)
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
	uid := params.User(id.Id())

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
	offerDetails := applicationOfferDocToDetails(uid, &offer)

	return &offerDetails, nil
}

// applicationOfferDocToDetails returns a jujuparams structure based on the provided
// application offer mongo doc.
func applicationOfferDocToDetails(id params.User, offerDoc *mongodoc.ApplicationOffer) jujuparams.ApplicationOfferAdminDetails {
	access := offerDoc.Users[mongodoc.User(id)]

	endpoints := make([]jujuparams.RemoteEndpoint, len(offerDoc.Endpoints))
	for i, endpoint := range offerDoc.Endpoints {
		endpoints[i] = jujuparams.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      charm.RelationRole(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	spaces := make([]jujuparams.RemoteSpace, len(offerDoc.Spaces))
	for i, space := range offerDoc.Spaces {
		spaces[i] = jujuparams.RemoteSpace{
			CloudType:          space.CloudType,
			Name:               space.Name,
			ProviderId:         space.ProviderId,
			ProviderAttributes: space.ProviderAttributes,
		}
	}
	var users []jujuparams.OfferUserDetails
	for u, ua := range offerDoc.Users {
		if access == mongodoc.ApplicationOfferAdminAccess || params.User(u) == id || u == identchecker.Everyone {
			userTag := conv.ToUserTag(params.User(u))
			users = append(users, jujuparams.OfferUserDetails{
				UserName:    userTag.Id(),
				DisplayName: userTag.Name(),
				Access:      ua.String(),
			})
		}
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].UserName < users[j].UserName
	})

	var connections []jujuparams.OfferConnection
	if access == mongodoc.ApplicationOfferAdminAccess {
		connections = make([]jujuparams.OfferConnection, len(offerDoc.Connections))
		for i, connection := range offerDoc.Connections {
			connections[i] = jujuparams.OfferConnection{
				SourceModelTag: connection.SourceModelTag,
				RelationId:     connection.RelationId,
				Username:       connection.Username,
				Endpoint:       connection.Endpoint,
				IngressSubnets: connection.IngressSubnets,
			}
		}
	}
	return jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(offerDoc.ModelUUID).String(),
			OfferUUID:              offerDoc.OfferUUID,
			OfferURL:               offerDoc.OfferURL,
			OfferName:              offerDoc.OfferName,
			ApplicationDescription: offerDoc.ApplicationDescription,
			Endpoints:              endpoints,
			Spaces:                 spaces,
			Bindings:               offerDoc.Bindings,
			Users:                  users,
		},
		ApplicationName: offerDoc.ApplicationName,
		CharmURL:        offerDoc.CharmURL,
		Connections:     connections,
	}
}

// GrantOfferAccess grants rights for an application offer.
func (j *JEM) GrantOfferAccess(ctx context.Context, id identchecker.ACLIdentity, user params.User, offerURL string, access jujuparams.OfferAccessPermission) (err error) {
	uid := params.User(id.Id())

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

	u := new(jimmdb.Update).Set(mongodoc.User(user).FieldName("users"), permission)
	if err := j.DB.UpdateApplicationOffer(ctx, &offer, u, true); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// RevokeOfferAccess revokes rights for an application offer.
func (j *JEM) RevokeOfferAccess(ctx context.Context, id identchecker.ACLIdentity, user params.User, offerURL string, access jujuparams.OfferAccessPermission) (err error) {
	uid := params.User(id.Id())

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

	// first revoke access in the jimm DB
	u := new(jimmdb.Update).Set(mongodoc.User(user).FieldName("users"), permission)
	if err := j.DB.UpdateApplicationOffer(ctx, &offer, u, true); err != nil {
		return errgo.Mask(err)
	}

	// then revoke on the actual controller
	conn, err := j.OpenAPI(ctx, offer.ControllerPath)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	err = conn.RevokeApplicationOfferAccess(ctx, offer.OfferURL, user, access)
	if err != nil {
		return errgo.Mask(err)
	}

	return nil
}

// DestroyOffer removes the application offer.
func (j *JEM) DestroyOffer(ctx context.Context, id identchecker.ACLIdentity, offerURL string, force bool) (err error) {
	uid := params.User(id.Id())

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

	// first remove application offer from the jimm DB
	err = j.DB.RemoveApplicationOffer(ctx, &offer)
	if err != nil {
		return errgo.Mask(err)
	}

	// then remove from the actual controller
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
	u.Set("connections", offerDetails.Connections)
	u.Set("endpoints", offerDetails.Endpoints)
	u.Set("offer-name", offerDetails.OfferName)
	u.Set("offer-url", offerDetails.OfferURL)

	return errgo.Mask(j.DB.UpdateApplicationOffer(ctx, &offer, u, false))
}

func offerDetailsToMongodoc(model *mongodoc.Model, offerDetails jujuparams.ApplicationOfferAdminDetails) mongodoc.ApplicationOffer {
	endpoints := make([]mongodoc.RemoteEndpoint, len(offerDetails.Endpoints))
	for i, endpoint := range offerDetails.Endpoints {
		endpoints[i] = mongodoc.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      string(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	users := make(map[mongodoc.User]mongodoc.ApplicationOfferAccessPermission, len(offerDetails.Users))
	for _, user := range offerDetails.Users {
		pu, err := conv.FromUserID(user.UserName)
		if err != nil {
			// If we can't parse the user, it's either a local user which
			// we don't store, or an invalid user which can't do anything.
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
			continue
		}
		users[mongodoc.User(pu)] = access
	}
	spaces := make([]mongodoc.RemoteSpace, len(offerDetails.Spaces))
	for i, space := range offerDetails.Spaces {
		spaces[i] = mongodoc.RemoteSpace{
			CloudType:          space.CloudType,
			Name:               space.Name,
			ProviderId:         space.ProviderId,
			ProviderAttributes: space.ProviderAttributes,
		}
	}
	connections := make([]mongodoc.OfferConnection, len(offerDetails.Connections))
	for i, connection := range offerDetails.Connections {
		connections[i] = mongodoc.OfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationId:     connection.RelationId,
			Username:       connection.Username,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
		}
	}

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
		Endpoints:              endpoints,
		Spaces:                 spaces,
		Bindings:               offerDetails.Bindings,
		Users:                  users,
		Connections:            connections,
	}
}
