// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// IdentityManager is an implementation of the jimm.IdentityManager interface.
type IdentityManager struct {
	FetchIdentity_   func(ctx context.Context, id string) (*openfga.User, error)
	ListIdentities_  func(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]openfga.User, error)
	CountIdentities_ func(ctx context.Context, user *openfga.User) (int, error)
}

func (i *IdentityManager) FetchIdentity(ctx context.Context, id string) (*openfga.User, error) {
	if i.FetchIdentity_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return i.FetchIdentity_(ctx, id)
}
func (i *IdentityManager) ListIdentities(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]openfga.User, error) {
	if i.ListIdentities_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return i.ListIdentities_(ctx, user, pagination, match)
}
func (i *IdentityManager) CountIdentities(ctx context.Context, user *openfga.User) (int, error) {
	if i.CountIdentities_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return i.CountIdentities_(ctx, user)
}
