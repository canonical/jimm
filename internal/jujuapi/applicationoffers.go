// Copyright 2020 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
)

func init() {
	facadeInit["ApplicationOffers"] = func(r *controllerRoot) []int {
		offerMethod := rpc.Method(r.Offer)
		getConsumeDetailsMethod := rpc.Method(r.GetConsumeDetails)
		getConsumeDetailsMethodV3 := rpc.Method(r.GetConsumeDetailsV3)
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

		r.AddMethod("ApplicationOffers", 3, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 3, "GetConsumeDetails", getConsumeDetailsMethodV3)
		r.AddMethod("ApplicationOffers", 3, "ListApplicationOffers", listOffersMethod)
		r.AddMethod("ApplicationOffers", 3, "ModifyOfferAccess", modifyOfferAccessMethod)
		r.AddMethod("ApplicationOffers", 3, "DestroyOffers", destroyOffersMethod)
		r.AddMethod("ApplicationOffers", 3, "FindApplicationOffers", findOffersMethod)
		r.AddMethod("ApplicationOffers", 3, "ApplicationOffers", applicationOffersMethod)

		r.AddMethod("ApplicationOffers", 4, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 4, "GetConsumeDetails", getConsumeDetailsMethodV3)
		r.AddMethod("ApplicationOffers", 4, "ListApplicationOffers", listOffersMethod)
		r.AddMethod("ApplicationOffers", 4, "ModifyOfferAccess", modifyOfferAccessMethod)
		r.AddMethod("ApplicationOffers", 4, "DestroyOffers", destroyOffersMethod)
		r.AddMethod("ApplicationOffers", 4, "FindApplicationOffers", findOffersMethod)
		r.AddMethod("ApplicationOffers", 4, "ApplicationOffers", applicationOffersMethod)

		return []int{1, 2, 3, 4}
	}
}

// Offer creates a new ApplicationOffer.
func (r *controllerRoot) Offer(ctx context.Context, args jujuparams.AddApplicationOffers) (jujuparams.ErrorResults, error) {
	result := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Offers)),
	}
	for i, addOfferParams := range args.Offers {
		result.Results[i].Error = mapError(r.offer(ctx, addOfferParams))
	}
	return result, nil
}

func (r *controllerRoot) offer(ctx context.Context, args jujuparams.AddApplicationOffer) error {
	const op = errors.Op("jujuapi.Offer")

	mt, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.E(op, errors.CodeBadRequest, err)
	}
	offerOwnerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errors.E(op, errors.CodeBadRequest, err)
	}
	err = r.jimm.Offer(ctx, r.user, jimm.AddApplicationOfferParams{
		ModelTag:               mt,
		OwnerTag:               offerOwnerTag,
		OfferName:              args.OfferName,
		ApplicationName:        args.ApplicationName,
		ApplicationDescription: args.ApplicationDescription,
		Endpoints:              args.Endpoints,
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GetConsumeDetails implements the GetConsumeDetails procedure of the
// ApplicationOffers facade (version 1 & 2).
func (r *controllerRoot) GetConsumeDetails(ctx context.Context, args jujuparams.OfferURLs) (jujuparams.ConsumeOfferDetailsResults, error) {
	results := jujuparams.ConsumeOfferDetailsResults{
		Results: make([]jujuparams.ConsumeOfferDetailsResult, len(args.OfferURLs)),
	}
	for i, offerURL := range args.OfferURLs {
		var err error
		results.Results[i].ConsumeOfferDetails, err = r.getConsumeDetails(ctx, r.user, args.BakeryVersion, offerURL)
		results.Results[i].Error = mapError(err)
	}
	return results, nil
}

// GetConsumeDetailsV3 implements the GetConsumeDetails procedure of the
// ApplicationOffers facade (version 3).
func (r *controllerRoot) GetConsumeDetailsV3(ctx context.Context, args jujuparams.ConsumeOfferDetailsArg) (jujuparams.ConsumeOfferDetailsResults, error) {
	results := jujuparams.ConsumeOfferDetailsResults{
		Results: make([]jujuparams.ConsumeOfferDetailsResult, len(args.OfferURLs.OfferURLs)),
	}

	user := r.user
	if args.UserTag != "" {
		var err error
		user, err = r.masquerade(ctx, args.UserTag)
		if err != nil {
			return jujuparams.ConsumeOfferDetailsResults{}, err
		}
	}

	for i, offerURL := range args.OfferURLs.OfferURLs {
		var err error
		results.Results[i].ConsumeOfferDetails, err = r.getConsumeDetails(ctx, user, args.OfferURLs.BakeryVersion, offerURL)
		results.Results[i].Error = mapError(err)
	}
	return results, nil
}

func (r *controllerRoot) getConsumeDetails(ctx context.Context, u *dbmodel.User, v bakery.Version, offerURL string) (jujuparams.ConsumeOfferDetails, error) {
	const op = errors.Op("jujuapi.GetConsumeDetails")

	ourl, err := crossmodel.ParseOfferURL(offerURL)
	if err != nil {
		return jujuparams.ConsumeOfferDetails{}, errors.E(op, "cannot parse offer URL", errors.CodeBadRequest, err)
	}

	// Ensure the path is normalised.
	if ourl.User == "" {
		// If the model owner is not specified use the specified user.
		ourl.User = u.Username
	}

	details := jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			OfferURL: ourl.AsLocal().Path(),
		},
	}
	if err := r.jimm.GetApplicationOfferConsumeDetails(ctx, u, &details, v); err != nil {
		return jujuparams.ConsumeOfferDetails{}, errors.E(op, err)
	}
	return details, nil
}

// ListApplicationOffers returns all offers matching the specified filters.
func (r *controllerRoot) ListApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResults, error) {
	const op = errors.Op("jujuapi.ListApplicationOffers")
	results := jujuparams.QueryApplicationOffersResults{}

	offers, err := r.jimm.ListApplicationOffers(ctx, r.user, args.Filters...)
	if err != nil {
		return results, errors.E(op, err)
	}
	results.Results = offers

	return results, nil
}

// FindApplicationOffers returns all offers matching the specified filters
// as long as the user has read access to each offer. It also omits details
// on users and connections.
func (r *controllerRoot) FindApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResults, error) {
	const op = errors.Op("jujuapi.FindApplicationOffers")
	results := jujuparams.QueryApplicationOffersResults{}

	offers, err := r.jimm.FindApplicationOffers(ctx, r.user, args.Filters...)
	if err != nil {
		return results, errors.E(op, err)
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
		results.Results[i].Error = mapError(r.modifyOfferAccess(ctx, change))
	}
	return results, nil
}

func (r *controllerRoot) modifyOfferAccess(ctx context.Context, change jujuparams.ModifyOfferAccess) error {
	const op = errors.Op("jujuapi.ModifyOfferAccess")

	ut, err := parseUserTag(change.UserTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	switch change.Action {
	case jujuparams.GrantOfferAccess:
		if err := r.jimm.GrantOfferAccess(ctx, r.user, change.OfferURL, ut, change.Access); err != nil {
			return errors.E(op, err)
		}
		return nil
	case jujuparams.RevokeOfferAccess:
		if err := r.jimm.RevokeOfferAccess(ctx, r.user, change.OfferURL, ut, change.Access); err != nil {
			return errors.E(op, err)
		}
		return nil
	default:
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("unknown action %q", change.Action))
	}
}

// DestroyOffers removes specified application offers.
func (r *controllerRoot) DestroyOffers(ctx context.Context, args jujuparams.DestroyApplicationOffers) (jujuparams.ErrorResults, error) {
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.OfferURLs)),
	}

	for i, offerURL := range args.OfferURLs {
		results.Results[i].Error = mapError(r.jimm.DestroyOffer(ctx, r.user, offerURL, args.Force))
	}
	return results, nil
}

func (r *controllerRoot) ApplicationOffers(ctx context.Context, args jujuparams.OfferURLs) (jujuparams.ApplicationOffersResults, error) {
	result := jujuparams.ApplicationOffersResults{
		Results: make([]jujuparams.ApplicationOfferResult, len(args.OfferURLs)),
	}
	for i, offerURL := range args.OfferURLs {
		details, err := r.jimm.GetApplicationOffer(ctx, r.user, offerURL)
		result.Results[i] = jujuparams.ApplicationOfferResult{
			Result: details,
			Error:  mapError(err),
		}
	}

	return result, nil
}
