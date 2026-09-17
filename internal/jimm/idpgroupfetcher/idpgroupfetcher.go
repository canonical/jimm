// Copyright 2026 Canonical.

package idpgroupfetcher

import (
	"context"

	"github.com/canonical/jimm/v3/internal/jimm/offer"
)

// Params configures an IdPGroupFetcher implementation.
type Params struct {
	// HookServiceAddress is the address of the hook-service gRPC API.
	// When empty, the NoOp fetcher is used.
	HookServiceAddress string
	// HookServiceToken is the Bearer token sent to the hook-service.
	HookServiceToken string
}

// New returns an offer.IdPGroupFetcher configured by the given params.
func New(ctx context.Context, p Params) (offer.IdPGroupFetcher, error) {
	if p.HookServiceAddress == "" {
		return NoOp{}, nil
	}
	return NewHookService(p.HookServiceAddress, p.HookServiceToken)
}

// NoOp resolves no groups for any user. It is used when no identity
// provider integration is configured.
type NoOp struct{}

// FetchGroups implements offer.IdPGroupFetcher.
func (NoOp) FetchGroups(ctx context.Context, username string) ([]string, error) {
	return nil, nil
}
