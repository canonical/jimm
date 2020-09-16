// Copyright 2020 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["ApplicationOffers"] = func(r *controllerRoot) []int {
		offerMethod := rpc.Method(r.Offer)
		getConsumeDetailsMethod := rpc.Method(r.GetConsumeDetails)
		listOffersMethod := rpc.Method(r.ListApplicationOffers)
		modifyOfferAccessMethod := rpc.Method(r.ModifyOfferAccess)
		destroyOffersMethod := rpc.Method(r.DestroyOffers)
		findOffersMethod := rpc.Method(r.FindApplicationOffers)
		applicationOffersMethod := rpc.Method(r.ApplicationOffers)

		r.AddMethod("ApplicationOffers", 1, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 1, "GetConsumeDetails", getConsumeDetailsMethod)
		r.AddMethod("ApplicationOffers", 1, "ListApplicationOffers", listOffersMethod)
		r.AddMethod("ApplicationOffers", 1, "ModifyOfferAccess", modifyOfferAccessMethod)
		r.AddMethod("ApplicationOffers", 1, "DestroyOffers", destroyOffersMethod)
		r.AddMethod("ApplicationOffers", 1, "FindApplicationOffers", findOffersMethod)
		r.AddMethod("ApplicationOffers", 1, "ApplicationOffers", applicationOffersMethod)

		r.AddMethod("ApplicationOffers", 2, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 2, "GetConsumeDetails", getConsumeDetailsMethod)
		r.AddMethod("ApplicationOffers", 2, "ListApplicationOffers", listOffersMethod)
		r.AddMethod("ApplicationOffers", 2, "ModifyOfferAccess", modifyOfferAccessMethod)
		r.AddMethod("ApplicationOffers", 2, "DestroyOffers", destroyOffersMethod)
		r.AddMethod("ApplicationOffers", 2, "FindApplicationOffers", findOffersMethod)
		r.AddMethod("ApplicationOffers", 2, "ApplicationOffers", applicationOffersMethod)

		return []int{1, 2}
	}
}

// Offer creates a new ApplicationOffer.
func (r *controllerRoot) Offer(ctx context.Context, args jujuparams.AddApplicationOffers) (jujuparams.ErrorResults, error) {
	result := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Offers)),
	}
	for i, addOfferParams := range args.Offers {
		result.Results[i].Error = mapError(r.jem.Offer(ctx, r.identity, addOfferParams))
	}
	return result, nil
}

// GetConsumeDetails implements the GetConsumeDetails procedure of the
// ApplicationOffers facade (version 1 & 2).
func (r *controllerRoot) GetConsumeDetails(ctx context.Context, args jujuparams.OfferURLs) (jujuparams.ConsumeOfferDetailsResults, error) {
	results := jujuparams.ConsumeOfferDetailsResults{
		Results: make([]jujuparams.ConsumeOfferDetailsResult, len(args.OfferURLs)),
	}
	for i, offerURL := range args.OfferURLs {
		ourl, err := crossmodel.ParseOfferURL(offerURL)
		if err != nil {
			results.Results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, "cannot parse offer URL"))
			continue
		}

		// Ensure the path is normalised.
		if ourl.User == "" {
			// If the model owner is not specified use the authenticated
			// user.
			ourl.User = conv.ToUserTag(params.User(r.identity.Id())).Id()
		}

		details := jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetails{
				OfferURL: ourl.AsLocal().Path(),
			},
		}

		if err := r.jem.GetApplicationOfferConsumeDetails(ctx, r.identity, &details, args.BakeryVersion); err != nil {
			results.Results[i].Error = mapError(err)
		} else {
			results.Results[i].ConsumeOfferDetails = details
		}
	}
	return results, nil
}

// ListApplicationOffers returns all offers matching the specified filters.
func (r *controllerRoot) ListApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResults, error) {
	results := jujuparams.QueryApplicationOffersResults{}

	offers, err := r.jem.ListApplicationOffers(ctx, r.identity, args.Filters...)
	if err != nil {
		return results, errgo.Mask(err)
	}
	results.Results = offers

	return results, nil
}

// FindApplicationOffers returns all offers matching the specified filters
// as long as the user has read access to each offer. It also omits details
// on users and connections.
func (r *controllerRoot) FindApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResults, error) {
	results := jujuparams.QueryApplicationOffersResults{}

	offers, err := r.jem.FindApplicationOffers(ctx, r.identity, args.Filters...)
	if err != nil {
		return results, errgo.Mask(err)
	}
	results.Results = offers

	return results, nil
}

// ModifyOfferAccess modifies application offer access.
func (r *controllerRoot) ModifyOfferAccess(ctx context.Context, args jujuparams.ModifyOfferAccessRequest) (jujuparams.ErrorResults, error) {
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Changes)),
	}

	for i, change := range args.Changes {
		results.Results[i].Error = mapError(r.modifyOfferAcces(ctx, change))
	}
	return results, nil
}

func (r *controllerRoot) modifyOfferAcces(ctx context.Context, change jujuparams.ModifyOfferAccess) error {
	userTag, err := names.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	user, err := conv.FromUserTag(userTag)
	if err != nil {
		return errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}
	switch change.Action {
	case jujuparams.GrantOfferAccess:
		return errgo.Mask(r.jem.GrantOfferAccess(ctx, r.identity, user, change.OfferURL, change.Access), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrBadRequest))
	case jujuparams.RevokeOfferAccess:
		return errgo.Mask(r.jem.RevokeOfferAccess(ctx, r.identity, user, change.OfferURL, change.Access), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrBadRequest))
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "unknown action %q", change.Action)
	}
}

// DestroyOffers removes specified application offers.
func (r *controllerRoot) DestroyOffers(ctx context.Context, args jujuparams.DestroyApplicationOffers) (jujuparams.ErrorResults, error) {
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.OfferURLs)),
	}

	for i, offerURL := range args.OfferURLs {
		results.Results[i].Error = mapError(r.jem.DestroyOffer(ctx, r.identity, offerURL, args.Force))
	}
	return results, nil
}

func (r *controllerRoot) ApplicationOffers(ctx context.Context, args jujuparams.OfferURLs) (jujuparams.ApplicationOffersResults, error) {
	result := jujuparams.ApplicationOffersResults{
		Results: make([]jujuparams.ApplicationOfferResult, len(args.OfferURLs)),
	}
	for i, offerURL := range args.OfferURLs {
		details, err := r.jem.GetApplicationOffer(ctx, r.identity, offerURL)
		result.Results[i] = jujuparams.ApplicationOfferResult{
			Result: details,
			Error:  mapError(err),
		}
	}

	return result, nil
}
