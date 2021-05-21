// Copyright 2020 Canonical Ltd.

package apiconn

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/params"
)

// Offer creates a new ApplicationOffer on the controller. Offer uses the
// Offer procedure on the ApplicationOffers facade version 2.
func (c *Conn) Offer(ctx context.Context, offer jujuparams.AddApplicationOffer) error {
	args := jujuparams.AddApplicationOffers{
		Offers: []jujuparams.AddApplicationOffer{offer},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("ApplicationOffers", 2, "", "Offer", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// ListApplicationOffers lists ApplicationOffers on the controller matching
// the given filters. ListApplicationOffers uses the ListApplicationOffers
// procedure on the ApplicationOffers facade version 2.
func (c *Conn) ListApplicationOffers(ctx context.Context, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	args := jujuparams.OfferFilters{
		Filters: filters,
	}

	var resp jujuparams.QueryApplicationOffersResults
	err := c.APICall("ApplicationOffers", 2, "", "ListApplicationOffers", &args, &resp)
	if err != nil {
		return nil, newAPIError(err)
	}
	return resp.Results, nil
}

// FindApplicationOffers finds ApplicationOffers on the controller matching
// the given filters. FindApplicationOffers uses the FindApplicationOffers
// procedure on the ApplicationOffers facade version 2.
func (c *Conn) FindApplicationOffers(ctx context.Context, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
	args := jujuparams.OfferFilters{
		Filters: filters,
	}

	var resp jujuparams.QueryApplicationOffersResults
	err := c.APICall("ApplicationOffers", 2, "", "FindApplicationOffers", &args, &resp)
	if err != nil {
		return nil, newAPIError(err)
	}
	return resp.Results, nil
}

// GetApplicationOffer retrives the details of the specified
// ApplicationOffer. The given ApplicationOfferAdminDetails must specify an
// OfferURL the rest of the structure will be filled in by the API request.
// GetApplicationOffer uses the ApplicationOffers procedure on the
// ApplicationOffers facade version 2.
func (c *Conn) GetApplicationOffer(ctx context.Context, info *jujuparams.ApplicationOfferAdminDetails) error {
	args := jujuparams.OfferURLs{
		OfferURLs: []string{info.OfferURL},
	}

	var resp jujuparams.ApplicationOffersResults
	err := c.APICall("ApplicationOffers", 2, "", "ApplicationOffers", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	if resp.Results[0].Result != nil {
		*info = *resp.Results[0].Result
	}
	return newAPIError(resp.Results[0].Error)
}

// GetApplicationOffers retrives the details of the multiple
// ApplicationOffers. Each of ApplicationOfferAdminDetails must specify an
// OfferURL the rest of the structure will be filled in by the API request.
// GetApplicationOffesr uses the ApplicationOffers procedure on the
// ApplicationOffers facade version 2.
func (c *Conn) GetApplicationOffers(ctx context.Context, infos []*jujuparams.ApplicationOfferAdminDetails) error {
	urls := make([]string, len(infos))
	for i, info := range infos {
		urls[i] = info.OfferURL
	}
	args := jujuparams.OfferURLs{
		OfferURLs: urls,
	}

	var resp jujuparams.ApplicationOffersResults
	err := c.APICall("ApplicationOffers", 2, "", "ApplicationOffers", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != len(infos) {
		return errgo.Newf("unexpected number of results (expected %d, got %d)", len(infos), len(resp.Results))
	}
	var firstError error
	for i, _ := range infos {
		if resp.Results[i].Error != nil && firstError == nil {
			firstError = newAPIError(resp.Results[i].Error)
		}
		if resp.Results[i].Result != nil {
			*infos[i] = *resp.Results[i].Result
		}
	}
	return firstError
}

// GrantApplicationOfferAccess grants the specified permission to the
// given user on the given application offer. GrantApplicationOfferAccess
// uses the ModifyOfferAccess procedure on the ApplicationOffers facade
// version 2.
func (c *Conn) GrantApplicationOfferAccess(ctx context.Context, offerURL string, user params.User, access jujuparams.OfferAccessPermission) error {
	args := jujuparams.ModifyOfferAccessRequest{
		Changes: []jujuparams.ModifyOfferAccess{{
			UserTag:  conv.ToUserTag(user).String(),
			Action:   jujuparams.GrantOfferAccess,
			Access:   access,
			OfferURL: offerURL,
		}},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("ApplicationOffers", 2, "", "ModifyOfferAccess", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// RevokeApplicationOfferAccess revokes the specified permission from the
// given user on the given application offer. RevokeApplicationOfferAccess
// uses the ModifyOfferAccess procedure on the ApplicationOffers facade
// version 1.
func (c *Conn) RevokeApplicationOfferAccess(ctx context.Context, offerURL string, user params.User, access jujuparams.OfferAccessPermission) error {
	args := jujuparams.ModifyOfferAccessRequest{
		Changes: []jujuparams.ModifyOfferAccess{{
			UserTag:  conv.ToUserTag(user).String(),
			Action:   jujuparams.RevokeOfferAccess,
			Access:   access,
			OfferURL: offerURL,
		}},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("ApplicationOffers", 2, "", "ModifyOfferAccess", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// DestroyApplicationOffer destroys the given application offer.
// DestroyApplicationOffer uses the DestroyOffers procedure
// from the ApplicationOffers facade version 2.
func (c *Conn) DestroyApplicationOffer(ctx context.Context, offer string, force bool) error {
	args := jujuparams.DestroyApplicationOffers{
		OfferURLs: []string{offer},
		Force:     force,
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("ApplicationOffers", 2, "", "DestroyOffers", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// GetApplicationOfferConsumeDetails retrieves the details needed to
// consume an application offer. The given ConsumeOfferDetails structure
// must include an Offer.OfferURL and the rest of the structure will be
// filled in by the API call. GetApplicationOfferConsumeDetails uses the
// GetConsumeDetails procedure on the ApplicationOffers facade version 3.
func (c *Conn) GetApplicationOfferConsumeDetails(ctx context.Context, user params.User, info *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	args := jujuparams.ConsumeOfferDetailsArg{
		OfferURLs: jujuparams.OfferURLs{
			OfferURLs:     []string{info.Offer.OfferURL},
			BakeryVersion: v,
		},
		UserTag: conv.ToUserTag(user).String(),
	}

	var resp jujuparams.ConsumeOfferDetailsResults
	err := c.APICall("ApplicationOffers", 3, "", "GetConsumeDetails", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	if resp.Results[0].Error != nil {
		return newAPIError(resp.Results[0].Error)
	}
	*info = resp.Results[0].ConsumeOfferDetails
	return nil
}
