// Copyright 2020 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
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

		r.AddMethod("ApplicationOffers", 1, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 1, "GetConsumeDetails", getConsumeDetailsMethod)
		r.AddMethod("ApplicationOffers", 1, "ListApplicationOffers", listOffersMethod)

		r.AddMethod("ApplicationOffers", 2, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 2, "GetConsumeDetails", getConsumeDetailsMethod)
		r.AddMethod("ApplicationOffers", 2, "ListApplicationOffers", listOffersMethod)

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

func (r *controllerRoot) ListApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResults, error) {
	results := jujuparams.QueryApplicationOffersResults{}

	offers, err := r.jem.ListApplicationOffers(ctx, r.identity, args.Filters...)
	if err != nil {
		return results, errgo.Mask(err)
	}
	results.Results = offers

	return results, nil
}
