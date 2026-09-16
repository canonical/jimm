// Copyright 2025 Canonical.

// The idpgroupfetcher package provides implementations of the
// offer.IdPGroupFetcher interface.
package idpgroupfetcher

import (
	"context"

	"github.com/canonical/jimm/v3/internal/jimm/offer"
)

// Params holds parameters needed to configure an IdPGroupFetcher
// implementation.
type Params struct{}

// New returns an offer.IdPGroupFetcher configured by the given params.
//
// TODO: construct the identity provider implementation from p when the
// integration lands.
func New(ctx context.Context, p Params) (offer.IdPGroupFetcher, error) {
	return NoOp{}, nil
}

// NoOp is an offer.IdPGroupFetcher implementation that resolves no
// groups for any user. It is used when no identity provider
// integration is configured: group-derived access is denied in
// non-session flows.
type NoOp struct{}

// FetchGroups implements offer.IdPGroupFetcher.
func (NoOp) FetchGroups(ctx context.Context, username string) ([]string, error) {
	return nil, nil
}
