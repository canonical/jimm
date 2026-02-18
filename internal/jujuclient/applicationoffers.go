// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
)

type OfferParams struct {
	ModelUUID   string
	Application string
	Endpoints   []string
	Owner       string
	OfferName   string
	Desc        string
}

// Offer creates a new ApplicationOffer on the controller. Offer uses the
// Offer procedure on the ApplicationOffers facade.
func (c Connection) Offer(ctx context.Context, offer OfferParams) error {
	appOfferAPI := applicationoffers.NewClient(&c)
	res, err := appOfferAPI.Offer(offer.ModelUUID, offer.Application, offer.Endpoints, offer.Owner, offer.OfferName, offer.Desc)
	if err != nil {
		return err
	}
	if len(res) > 0 && res[0].Error != nil {
		return res[0].Error
	}
	return nil
}

// ListApplicationOffers lists ApplicationOffers on the controller matching
// the given filters. ListApplicationOffers uses the ListApplicationOffers
// procedure on the ApplicationOffers facade.
func (c Connection) ListApplicationOffers(ctx context.Context, filters []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	appOfferAPI := applicationoffers.NewClient(&c)
	return appOfferAPI.ListOffers(filters...)
}

// FindApplicationOffers finds ApplicationOffers on the controller matching
// the given filters. FindApplicationOffers uses the FindApplicationOffers
// procedure on the ApplicationOffers facade.
func (c Connection) FindApplicationOffers(ctx context.Context, filters []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	appOfferAPI := applicationoffers.NewClient(&c)
	return appOfferAPI.FindApplicationOffers(filters...)
}

// GetApplicationOffer retrives the details of the specified
// ApplicationOffer. The given ApplicationOfferAdminDetails must specify an
// OfferURL the rest of the structure will be filled in by the API request.
// GetApplicationOffer uses the ApplicationOffers procedure on the
// ApplicationOffers facade.
func (c Connection) GetApplicationOffer(ctx context.Context, urlStr string) (*crossmodel.ApplicationOfferDetails, error) {
	appOfferAPI := applicationoffers.NewClient(&c)
	return appOfferAPI.ApplicationOffer(urlStr)
}

// DestroyApplicationOffer destroys the given application offer.
// DestroyApplicationOffer uses the DestroyOffers procedure
// from the ApplicationOffers facade.
func (c Connection) DestroyApplicationOffer(ctx context.Context, offerURL string, force bool) error {
	appOfferAPI := applicationoffers.NewClient(&c)
	return appOfferAPI.DestroyOffers(force, offerURL)
}

// GetApplicationOfferConsumeDetails retrieves the details needed to
// consume an application offer. The given ConsumeOfferDetails structure
// must include an Offer.OfferURL and the rest of the structure will be
// filled in by the API call. GetApplicationOfferConsumeDetails uses the
// GetConsumeDetails procedure on the ApplicationOffers facade.
func (c Connection) GetApplicationOfferConsumeDetails(ctx context.Context, url string) (jujuparams.ConsumeOfferDetails, error) {
	appOfferAPI := applicationoffers.NewClient(&c)
	return appOfferAPI.GetConsumeDetails(url)
}
