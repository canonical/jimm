// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
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
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
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

	model, err := j.DB.ModelFromUUID(ctx, modelTag.Id())
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// The model owner is currently always an admin.
	if err = auth.CheckIsUser(ctx, id, model.Path.User); err != nil {
		if err = auth.CheckACL(ctx, id, model.ACL.Admin); err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
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

	endpoints := make([]mongodoc.RemoteEndpoint, len(offerDetails.Endpoints))
	for i, endpoint := range offerDetails.Endpoints {
		endpoints[i] = mongodoc.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      string(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	users := make([]mongodoc.OfferUserDetails, len(offerDetails.Users))
	for i, user := range offerDetails.Users {
		users[i] = mongodoc.OfferUserDetails{
			UserName:    user.UserName,
			DisplayName: user.DisplayName,
			Access:      user.Access,
		}
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

	doc := mongodoc.ApplicationOffer{
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

	err = j.DB.AddApplicationOffer(ctx, &doc)
	if err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return nil
		}
		return errgo.Mask(err)
	}

	for _, user := range offerDetails.Users {
		var userAccess mongodoc.ApplicationOfferAccessPermission
		zapctx.Debug(ctx, "adding user", zap.String("user", user.UserName))

		uid, err := conv.FromUserID(user.UserName)
		if err != nil {
			zapctx.Warn(ctx, "ignoring unsupported user name", zap.String("user name", user.UserName), zap.Error(err))
			continue
		}
		switch user.Access {
		case "read":
			userAccess = mongodoc.ApplicationOfferReadAccess
		case "consumer":
			userAccess = mongodoc.ApplicationOfferConsumeAccess
		case "admin":
			userAccess = mongodoc.ApplicationOfferAdminAccess
		default:
			zapctx.Warn(ctx, "unknown user access level", zap.String("level", user.Access))
			continue

		}
		offerAccess := mongodoc.ApplicationOfferAccess{
			User:      uid,
			OfferUUID: offerDetails.OfferUUID,
			Access:    userAccess,
		}
		err = j.DB.SetApplicationOfferAccess(ctx, offerAccess)
		if err != nil {
			return errgo.Mask(err)
		}
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
	access, err := j.DB.GetApplicationOfferAccess(ctx, uid, offer.OfferUUID)
	if err != nil {
		return errgo.Mask(err)
	}
	if access < mongodoc.ApplicationOfferConsumeAccess {
		// If the current user doesn't have access then check if it is
		// publicly available.
		access, err = j.DB.GetApplicationOfferAccess(ctx, params.User("everyone"), offer.OfferUUID)
		if err != nil {
			return errgo.Mask(err)
		}
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
	ctl, err := j.DB.Controller(ctx, offer.ControllerPath)
	if err != nil {
		return errgo.Mask(err)
	}
	conn, err := j.OpenAPIFromDoc(ctx, ctl)
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
func (j *JEM) ListApplicationOffers(ctx context.Context, id identchecker.ACLIdentity, filters ...jujuparams.OfferFilter) (_ []jujuparams.ApplicationOfferAdminDetails, err error) {
	uid := params.User(id.Id())

	docs, err := j.DB.ListApplicationOffers(ctx, filters)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	offers := []jujuparams.ApplicationOfferAdminDetails{}
	for _, offerDoc := range docs {
		access, err := j.DB.GetApplicationOfferAccess(ctx, uid, offerDoc.OfferUUID)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		if access == mongodoc.ApplicationOfferAdminAccess {
			offers = append(offers, applicationOfferDocToDetails(uid, &offerDoc))
		}
	}

	return offers, nil
}

// applicationOfferDocToDetails returns a jujuparams structure based on the provided
// application offer mongo doc.
func applicationOfferDocToDetails(id params.User, offerDoc *mongodoc.ApplicationOffer) jujuparams.ApplicationOfferAdminDetails {
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
	users := make([]jujuparams.OfferUserDetails, len(offerDoc.Users))
	for i, user := range offerDoc.Users {
		users[i] = jujuparams.OfferUserDetails{
			UserName:    user.UserName,
			DisplayName: user.DisplayName,
			Access:      user.Access,
		}
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].UserName < users[j].UserName
	})
	users = filterApplicationOfferUsers(id, mongodoc.ApplicationOfferAdminAccess, users)

	connections := make([]jujuparams.OfferConnection, len(offerDoc.Connections))
	for i, connection := range offerDoc.Connections {
		connections[i] = jujuparams.OfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationId:     connection.RelationId,
			Username:       connection.Username,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
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
	offerAccess, err := j.DB.GetApplicationOfferAccess(ctx, uid, offer.OfferUUID)
	if err != nil {
		return errgo.Mask(err)
	}

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

	// then grant access in the jimm db
	err = j.DB.SetApplicationOfferAccess(ctx, mongodoc.ApplicationOfferAccess{
		User:      user,
		OfferUUID: offer.OfferUUID,
		Access:    permission,
	})
	if err != nil {
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
	offerAccess, err := j.DB.GetApplicationOfferAccess(ctx, uid, offer.OfferUUID)
	if err != nil {
		return errgo.Mask(err)
	}

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
	err = j.DB.SetApplicationOfferAccess(ctx, mongodoc.ApplicationOfferAccess{
		User:      user,
		OfferUUID: offer.OfferUUID,
		Access:    permission,
	})
	if err != nil {
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
