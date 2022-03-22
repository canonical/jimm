// Copyright 2020 Canonical Ltd.

package jujuclient

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// Offer creates a new ApplicationOffer on the controller. Offer uses the
// Offer procedure on the ApplicationOffers facade version 2.
func (c Connection) Offer(ctx context.Context, offer jujuparams.AddApplicationOffer) error {
	const op = errors.Op("jujuclient.Offer")
	args := jujuparams.AddApplicationOffers{
		Offers: []jujuparams.AddApplicationOffer{offer},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "Offer", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// ListApplicationOffers lists ApplicationOffers on the controller matching
// the given filters. ListApplicationOffers uses the ListApplicationOffers
// procedure on the ApplicationOffers facade version 2.
func (c Connection) ListApplicationOffers(ctx context.Context, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jujuclient.ListApplicationOffers")
	args := jujuparams.OfferFilters{
		Filters: filters,
	}

	var resp jujuparams.QueryApplicationOffersResults
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "ListApplicationOffers", &args, &resp)
	if err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	return resp.Results, nil
}

// FindApplicationOffers finds ApplicationOffers on the controller matching
// the given filters. FindApplicationOffers uses the FindApplicationOffers
// procedure on the ApplicationOffers facade version 2.
func (c Connection) FindApplicationOffers(ctx context.Context, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	const op = errors.Op("jujuclient.FindApplicationOffers")
	args := jujuparams.OfferFilters{
		Filters: filters,
	}

	var resp jujuparams.QueryApplicationOffersResults
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "FindApplicationOffers", &args, &resp)
	if err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	return resp.Results, nil
}

// GetApplicationOffer retrives the details of the specified
// ApplicationOffer. The given ApplicationOfferAdminDetails must specify an
// OfferURL the rest of the structure will be filled in by the API request.
// GetApplicationOffer uses the ApplicationOffers procedure on the
// ApplicationOffers facade version 2.
func (c Connection) GetApplicationOffer(ctx context.Context, info *jujuparams.ApplicationOfferAdminDetails) error {
	const op = errors.Op("jujuclient.GetApplicationOffer")
	args := jujuparams.OfferURLs{
		OfferURLs: []string{info.OfferURL},
	}

	resp := jujuparams.ApplicationOffersResults{
		Results: make([]jujuparams.ApplicationOfferResult, 1),
	}
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "ApplicationOffers", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	*info = *resp.Results[0].Result
	return nil
}

// GrantApplicationOfferAccess grants the specified permission to the
// given user on the given application offer. GrantApplicationOfferAccess
// uses the ModifyOfferAccess procedure on the ApplicationOffers facade
// version 2.
func (c Connection) GrantApplicationOfferAccess(ctx context.Context, offerURL string, user names.UserTag, access jujuparams.OfferAccessPermission) error {
	const op = errors.Op("jujuclient.GrantApplicationOfferAccess")
	args := jujuparams.ModifyOfferAccessRequest{
		Changes: []jujuparams.ModifyOfferAccess{{
			UserTag:  user.String(),
			Action:   jujuparams.GrantOfferAccess,
			Access:   access,
			OfferURL: offerURL,
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "ModifyOfferAccess", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// RevokeApplicationOfferAccess revokes the specified permission from the
// given user on the given application offer. RevokeApplicationOfferAccess
// uses the ModifyOfferAccess procedure on the ApplicationOffers facade
// version 1.
func (c Connection) RevokeApplicationOfferAccess(ctx context.Context, offerURL string, user names.UserTag, access jujuparams.OfferAccessPermission) error {
	const op = errors.Op("jujuclient.RevokeApplicationOfferAccess")
	args := jujuparams.ModifyOfferAccessRequest{
		Changes: []jujuparams.ModifyOfferAccess{{
			UserTag:  user.String(),
			Action:   jujuparams.RevokeOfferAccess,
			Access:   access,
			OfferURL: offerURL,
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "ModifyOfferAccess", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// DestroyApplicationOffer destroys the given application offer.
// DestroyApplicationOffer uses the DestroyOffers procedure
// from the ApplicationOffers facade version 2.
func (c Connection) DestroyApplicationOffer(ctx context.Context, offer string, force bool) error {
	const op = errors.Op("jujuclient.DestroyApplicationOffer")
	args := jujuparams.DestroyApplicationOffers{
		OfferURLs: []string{offer},
		Force:     force,
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ApplicationOffers", 2, "", "DestroyOffers", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// GetApplicationOfferConsumeDetails retrieves the details needed to
// consume an application offer. The given ConsumeOfferDetails structure
// must include an Offer.OfferURL and the rest of the structure will be
// filled in by the API call. GetApplicationOfferConsumeDetails uses the
// GetConsumeDetails procedure on the ApplicationOffers facade version 3.
func (c Connection) GetApplicationOfferConsumeDetails(ctx context.Context, user names.UserTag, info *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	const op = errors.Op("jujuclient.GetApplicationOfferConsumeDetails")
	args := jujuparams.ConsumeOfferDetailsArg{
		OfferURLs: jujuparams.OfferURLs{
			OfferURLs:     []string{info.Offer.OfferURL},
			BakeryVersion: v,
		},
		UserTag: user.String(),
	}

	resp := jujuparams.ConsumeOfferDetailsResults{
		Results: make([]jujuparams.ConsumeOfferDetailsResult, 1),
	}
	err := c.client.Call(ctx, "ApplicationOffers", 3, "", "GetConsumeDetails", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	*info = resp.Results[0].ConsumeOfferDetails
	return nil
}
