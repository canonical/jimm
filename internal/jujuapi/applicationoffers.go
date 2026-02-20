// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"fmt"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/core/crossmodel"
	jujustatus "github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	"github.com/canonical/jimm/v3/internal/openfga"
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

		r.AddMethod("ApplicationOffers", 5, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 5, "GetConsumeDetails", getConsumeDetailsMethod)
		r.AddMethod("ApplicationOffers", 5, "ListApplicationOffers", listOffersMethod)
		r.AddMethod("ApplicationOffers", 5, "ModifyOfferAccess", modifyOfferAccessMethod)
		r.AddMethod("ApplicationOffers", 5, "DestroyOffers", destroyOffersMethod)
		r.AddMethod("ApplicationOffers", 5, "FindApplicationOffers", findOffersMethod)
		r.AddMethod("ApplicationOffers", 5, "ApplicationOffers", applicationOffersMethod)

		return []int{5}
	}
}

// Offer creates a new ApplicationOffer.
func (r *controllerRoot) Offer(ctx context.Context, args jujuparams.AddApplicationOffers) (jujuparams.ErrorResults, error) {
	result := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Offers)),
	}
	for i, addOfferParams := range args.Offers {
		result.Results[i].Error = r.mapError(ctx, r.offer(ctx, addOfferParams))
	}
	return result, nil
}

func (r *controllerRoot) offer(ctx context.Context, args jujuparams.AddApplicationOffer) error {

	mt, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.E(errors.CodeBadRequest, err)
	}
	offerOwnerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errors.E(errors.CodeBadRequest, err)
	}
	err = r.jimm.JujuManager().Offer(ctx, r.user, juju.AddApplicationOfferParams{
		ModelTag:               mt,
		OwnerTag:               offerOwnerTag,
		OfferName:              args.OfferName,
		ApplicationName:        args.ApplicationName,
		ApplicationDescription: args.ApplicationDescription,
		Endpoints:              args.Endpoints,
	})
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// GetConsumeDetails implements the GetConsumeDetails procedure of the
// ApplicationOffers facade.
func (r *controllerRoot) GetConsumeDetails(ctx context.Context, args jujuparams.ConsumeOfferDetailsArg) (jujuparams.ConsumeOfferDetailsResults, error) {
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
		results.Results[i].Error = r.mapError(ctx, err)
	}
	return results, nil
}

func (r *controllerRoot) getConsumeDetails(ctx context.Context, user *openfga.User, v bakery.Version, offerURL string) (jujuparams.ConsumeOfferDetails, error) {

	ourl, err := crossmodel.ParseOfferURL(offerURL)
	if err != nil {
		return jujuparams.ConsumeOfferDetails{}, errors.E("cannot parse offer URL", errors.CodeBadRequest, err)
	}

	// Ensure the path is normalised.
	if ourl.User == "" {
		// If the model owner is not specified use the specified user.
		ourl.User = user.Name
	}

	details := jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetailsV5{
			OfferURL: ourl.AsLocal().Path(),
		},
	}
	if err := r.jimm.JujuManager().GetApplicationOfferConsumeDetails(ctx, user, &details, v); err != nil {
		return jujuparams.ConsumeOfferDetails{}, errors.E(err)
	}
	return details, nil
}

// ListApplicationOffers returns all offers matching the specified filters.
func (r *controllerRoot) ListApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResultsV5, error) {

	results := jujuparams.QueryApplicationOffersResultsV5{}
	filters := filtersToCrossmodel(args.Filters)

	offers, err := r.jimm.JujuManager().ListApplicationOffers(ctx, r.user, filters...)
	if err != nil {
		return results, errors.E(err)
	}
	results.Results = offersToParams(offers)

	return results, nil
}

// FindApplicationOffers returns all offers matching the specified filters
// as long as the user has read access to each offer. It also omits details
// on users and connections.
func (r *controllerRoot) FindApplicationOffers(ctx context.Context, args jujuparams.OfferFilters) (jujuparams.QueryApplicationOffersResultsV5, error) {

	results := jujuparams.QueryApplicationOffersResultsV5{}
	filters := filtersToCrossmodel(args.Filters)

	offers, err := r.jimm.JujuManager().FindApplicationOffers(ctx, r.user, filters...)
	if err != nil {
		return results, errors.E(err)
	}
	results.Results = offersToParams(offers)

	return results, nil
}

// ModifyOfferAccess modifies application offer access.
func (r *controllerRoot) ModifyOfferAccess(ctx context.Context, args jujuparams.ModifyOfferAccessRequest) (jujuparams.ErrorResults, error) {
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Changes)),
	}

	for i, change := range args.Changes {
		results.Results[i].Error = r.mapError(ctx, r.modifyOfferAccess(ctx, change))
	}
	return results, nil
}

func (r *controllerRoot) modifyOfferAccess(ctx context.Context, change jujuparams.ModifyOfferAccess) error {

	ut, err := parseUserTag(change.UserTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}
	switch change.Action {
	case jujuparams.GrantOfferAccess:
		if err := r.jimm.PermissionManager().GrantOfferAccess(ctx, r.user, change.OfferURL, ut, change.Access); err != nil {
			return errors.E(err)
		}
		return nil
	case jujuparams.RevokeOfferAccess:
		if err := r.jimm.PermissionManager().RevokeOfferAccess(ctx, r.user, change.OfferURL, ut, change.Access); err != nil {
			return errors.E(err)
		}
		return nil
	default:
		return errors.E(errors.CodeBadRequest, fmt.Sprintf("unknown action %q", change.Action))
	}
}

// DestroyOffers removes specified application offers.
func (r *controllerRoot) DestroyOffers(ctx context.Context, args jujuparams.DestroyApplicationOffers) (jujuparams.ErrorResults, error) {
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.OfferURLs)),
	}

	for i, offerURL := range args.OfferURLs {
		results.Results[i].Error = r.mapError(ctx, r.jimm.JujuManager().DestroyOffer(ctx, r.user, offerURL, args.Force))
	}
	return results, nil
}

func (r *controllerRoot) ApplicationOffers(ctx context.Context, args jujuparams.OfferURLs) (jujuparams.ApplicationOffersResults, error) {
	result := jujuparams.ApplicationOffersResults{
		Results: make([]jujuparams.ApplicationOfferResult, len(args.OfferURLs)),
	}
	for i, offerURL := range args.OfferURLs {
		details, err := r.jimm.JujuManager().GetApplicationOffer(ctx, r.user, offerURL)
		if err != nil {
			result.Results[i].Error = r.mapError(ctx, err)
			continue
		}
		res := offerToParams(details)
		result.Results[i] = jujuparams.ApplicationOfferResult{
			Result: &res,
		}
	}

	return result, nil
}

func filtersToCrossmodel(filters []jujuparams.OfferFilter) []crossmodel.ApplicationOfferFilter {
	result := make([]crossmodel.ApplicationOfferFilter, len(filters))
	for i, f := range filters {
		result[i] = crossmodel.ApplicationOfferFilter{
			OwnerName:              f.OwnerName,
			ModelName:              f.ModelName,
			OfferName:              f.OfferName,
			ApplicationName:        f.ApplicationName,
			ApplicationDescription: f.ApplicationDescription,
			Endpoints:              make([]crossmodel.EndpointFilterTerm, len(f.Endpoints)),
			ConnectedUsers:         make([]string, 0, len(f.ConnectedUserTags)),
			AllowedConsumers:       make([]string, 0, len(f.AllowedConsumerTags)),
		}
		for j, endpoint := range f.Endpoints {
			result[i].Endpoints[j] = crossmodel.EndpointFilterTerm{
				Name:      endpoint.Name,
				Interface: endpoint.Interface,
				Role:      endpoint.Role,
			}
		}
		for _, userTag := range f.ConnectedUserTags {
			if user, err := names.ParseUserTag(userTag); err == nil {
				result[i].ConnectedUsers = append(result[i].ConnectedUsers, user.Id())
				continue
			}
			result[i].ConnectedUsers = append(result[i].ConnectedUsers, userTag)
		}
		for _, userTag := range f.AllowedConsumerTags {
			if user, err := names.ParseUserTag(userTag); err == nil {
				result[i].AllowedConsumers = append(result[i].AllowedConsumers, user.Id())
				continue
			}
			result[i].AllowedConsumers = append(result[i].AllowedConsumers, userTag)
		}
	}
	return result
}

func offersToParams(offers []*crossmodel.ApplicationOfferDetails) []jujuparams.ApplicationOfferAdminDetailsV5 {
	result := make([]jujuparams.ApplicationOfferAdminDetailsV5, len(offers))
	for i, offer := range offers {
		result[i] = offerToParams(offer)
	}
	return result
}

func offerToParams(offer *crossmodel.ApplicationOfferDetails) jujuparams.ApplicationOfferAdminDetailsV5 {
	endpoints := make([]jujuparams.RemoteEndpoint, len(offer.Endpoints))
	for i, endpoint := range offer.Endpoints {
		endpoints[i] = jujuparams.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      endpoint.Role,
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}

	users := make([]jujuparams.OfferUserDetails, len(offer.Users))
	for i, user := range offer.Users {
		users[i] = jujuparams.OfferUserDetails{
			UserName:    user.UserName,
			DisplayName: user.DisplayName,
			Access:      string(user.Access),
		}
	}

	connections := make([]jujuparams.OfferConnection, len(offer.Connections))
	for i, connection := range offer.Connections {
		connections[i] = jujuparams.OfferConnection{
			SourceModelTag: names.NewModelTag(connection.SourceModelUUID).String(),
			RelationId:     connection.RelationId,
			Username:       connection.Username,
			Endpoint:       connection.Endpoint,
			Status: jujuparams.EntityStatus{
				Status: jujustatus.Status(connection.Status),
				Info:   connection.Message,
				Since:  connection.Since,
			},
			IngressSubnets: connection.IngressSubnets,
		}
	}

	return jujuparams.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
			OfferName:              offer.OfferName,
			OfferURL:               offer.OfferURL,
			ApplicationDescription: offer.ApplicationDescription,
			Endpoints:              endpoints,
			Users:                  users,
		},
		ApplicationName: offer.ApplicationName,
		CharmURL:        offer.CharmURL,
		Connections:     connections,
	}
}
