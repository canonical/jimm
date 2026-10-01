// Copyright 2026 Canonical.

package idpgroupfetcher

import (
	"context"
	"fmt"

	"github.com/canonical/jimm/v3/internal/jimm/offer"
)

// Type identifies the IdP group fetcher implementation to construct.
type Type string

const (
	// TypeNone selects the NoOp fetcher, which resolves no groups for
	// any user: group-derived access is denied in non-session flows.
	// It is the zero value, so an unset type means no fetcher.
	TypeNone Type = ""
	// TypeHookService resolves IdP groups from the Canonical identity
	// platform's hook-service gRPC API.
	TypeHookService Type = "hook-service"
)

// Params configures an IdPGroupFetcher implementation.
type Params struct {
	// Type selects the IdP group fetcher implementation.
	Type Type
	// HookServiceAddress is the address of the hook-service gRPC API;
	// required when Type is TypeHookService.
	HookServiceAddress string
	// HookServiceToken is the Bearer token sent to the hook-service;
	// required when Type is TypeHookService.
	HookServiceToken string
}

// New returns an offer.IdPGroupFetcher configured by the given params.
// When Type is TypeNone (the zero value, e.g. when no identity provider
// integration is configured), it returns a NoOp fetcher rather than nil,
// so callers never need nil checks.
func New(p Params) (offer.IdPGroupFetcher, error) {
	switch p.Type {
	case TypeNone:
		return NoOp{}, nil
	case TypeHookService:
		return NewHookService(p.HookServiceAddress, p.HookServiceToken)
	default:
		return nil, fmt.Errorf("unknown idp group fetcher type %q", p.Type)
	}
}

// NoOp is an offer.IdPGroupFetcher that resolves no groups for any user.
// It is used when no identity provider integration is configured;
// group-derived access is denied in non-session flows.
type NoOp struct{}

// FetchGroups implements offer.IdPGroupFetcher.
func (NoOp) FetchGroups(ctx context.Context, username string) ([]string, error) {
	return nil, nil
}
