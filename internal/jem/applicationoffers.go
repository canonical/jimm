// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"sort"
	"strings"

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

	doc := mongodoc.ApplicationOffer{
		OfferUUID:              offerDetails.OfferUUID,
		OfferURL:               offerDetails.OfferURL,
		OwnerName:              conv.ToUserTag(model.Path.User).Id(),
		ModelName:              string(model.Path.Name),
		OfferName:              offerDetails.OfferName,
		ApplicationName:        offerDetails.ApplicationName,
		ApplicationDescription: offerDetails.ApplicationDescription,
		Endpoints:              offer.Endpoints,
		ControllerPath:         model.Controller,
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
