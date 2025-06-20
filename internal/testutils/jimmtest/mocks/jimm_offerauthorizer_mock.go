// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/juju/names/v5"
)

// OfferAuthorizer is a mock implementation of the OfferAuthorizer interface.
type OfferAuthorizer struct {
	// IsUserConsumerForOfferFunc is the mock function for IsUserConsumerForOffer.
	IsUserConsumerForOfferFunc func(ctx context.Context, userTag names.UserTag, offerTag names.ApplicationOfferTag) (bool, error)
}

// IsUserConsumerForOffer mocks the IsUserConsumerForOffer method.
func (m *OfferAuthorizer) IsUserConsumerForOffer(ctx context.Context, userTag names.UserTag, offerTag names.ApplicationOfferTag) (bool, error) {
	if m.IsUserConsumerForOfferFunc == nil {
		return false, nil
	}
	return m.IsUserConsumerForOfferFunc(ctx, userTag, offerTag)
}
