// Copyright 2020 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
)

func init() {
	facadeInit["ApplicationOffers"] = func(r *controllerRoot) []int {
		offerMethod := rpc.Method(r.Offer)

		r.AddMethod("ApplicationOffers", 1, "Offer", offerMethod)
		r.AddMethod("ApplicationOffers", 2, "Offer", offerMethod)

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
