// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"context"
	"fmt"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rebac_admin/utils"

	v1 "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

type identitiesService struct {
	jimm jujuapi.JIMM
}

func newidentitiesService(jimm jujuapi.JIMM) *identitiesService {
	return &identitiesService{
		jimm: jimm,
	}
}

// ListIdentities returns a page of Identity objects of at least `size` elements if available.
func (s *identitiesService) ListIdentities(ctx context.Context, params *resources.GetIdentitiesParams) (*resources.PaginatedResponse[resources.Identity], error) {
	raw, _ := v1.GetIdentityFromContext(ctx)
	user, _ := raw.(*openfga.User)

	count, err := s.jimm.CountIdentities(ctx, user)
	if err != nil {
		return nil, err
	}
	page, nextPage, pagination := pagination.CreatePagination(params.Size, params.Page, count)

	users, err := s.jimm.ListIdentities(ctx, user, pagination)
	if err != nil {
		return nil, err
	}
	rIdentities := make([]resources.Identity, len(users))
	for i, u := range users {
		rIdentities[i] = utils.FromUserToIdentity(u)
	}

	return &resources.PaginatedResponse[resources.Identity]{
		Data: rIdentities,
		Meta: resources.ResponseMeta{
			Page:  &page,
			Size:  len(rIdentities),
			Total: &count,
		},
		Next: resources.Next{
			Page: nextPage,
		},
	}, nil
}

// CreateIdentity creates a single Identity.
func (s *identitiesService) CreateIdentity(ctx context.Context, identity *resources.Identity) (*resources.Identity, error) {
	return nil, v1.NewNotImplementedError("create identity not implemented")
}

// GetIdentity returns a single Identity.
func (s *identitiesService) GetIdentity(ctx context.Context, identityId string) (*resources.Identity, error) {
	user, err := s.jimm.FetchIdentity(ctx, identityId)
	if err != nil {
		return nil, v1.NewNotFoundError(fmt.Sprintf("User with id %s not found", identityId))
	}
	identity := utils.FromUserToIdentity(*user)
	return &identity, nil
}

// UpdateIdentity updates an Identity.
func (s *identitiesService) UpdateIdentity(ctx context.Context, identity *resources.Identity) (*resources.Identity, error) {
	return nil, v1.NewNotImplementedError("update identity not implemented")
}

// // DeleteIdentity deletes an Identity.
func (s *identitiesService) DeleteIdentity(ctx context.Context, identityId string) (bool, error) {
	return false, v1.NewNotImplementedError("delete identity not implemented")
}

// GetIdentityGroups returns a page of Groups for identity `identityId`.
func (s *identitiesService) GetIdentityGroups(ctx context.Context, identityId string, params *resources.GetIdentitiesItemGroupsParams) (*resources.PaginatedResponse[resources.Group], error) {
	return nil, v1.NewNotImplementedError("delete identity not implemented")
}

// // PatchIdentityGroups performs addition or removal of a Group to/from an Identity.
func (s *identitiesService) PatchIdentityGroups(ctx context.Context, identityId string, groupPatches []resources.IdentityGroupsPatchItem) (bool, error) {
	return false, v1.NewNotImplementedError("delete identity not implemented")
}

// // GetIdentityRoles returns a page of Roles for identity `identityId`.
func (s *identitiesService) GetIdentityRoles(ctx context.Context, identityId string, params *resources.GetIdentitiesItemRolesParams) (*resources.PaginatedResponse[resources.Role], error) {
	return nil, v1.NewNotImplementedError("get identity roles not implemented")
}

// // PatchIdentityRoles performs addition or removal of a Role to/from an Identity.
func (s *identitiesService) PatchIdentityRoles(ctx context.Context, identityId string, rolePatches []resources.IdentityRolesPatchItem) (bool, error) {
	return false, v1.NewNotImplementedError("get identity roles not implemented")

}

// // GetIdentityEntitlements returns a page of Entitlements for identity `identityId`.
func (s *identitiesService) GetIdentityEntitlements(ctx context.Context, identityId string, params *resources.GetIdentitiesItemEntitlementsParams) (*resources.PaginatedResponse[resources.EntityEntitlement], error) {
	return nil, v1.NewNotImplementedError("get identity roles not implemented")
}

// // PatchIdentityEntitlements performs addition or removal of an Entitlement to/from an Identity.
func (s *identitiesService) PatchIdentityEntitlements(ctx context.Context, identityId string, entitlementPatches []resources.IdentityEntitlementsPatchItem) (bool, error) {
	return false, v1.NewNotImplementedError("get identity roles not implemented")
}
