// Copyright 2020 Canonical Ltd.

package conv

import (
	"github.com/juju/juju/core/crossmodel"

	"github.com/canonical/jimm/params"
)

// ToOfferURL creates an offer URL for the given model and offer name.
func ToOfferURL(model params.EntityPath, name string) string {
	offerURL := crossmodel.OfferURL{
		User:            ToUserTag(model.User).Id(),
		ModelName:       string(model.Name),
		ApplicationName: name,
	}
	return offerURL.Path()
}
