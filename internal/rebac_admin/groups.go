// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"context"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// groupsService implements the `GroupsService` interface.
type groupsService struct {
	jimm *jimm.JIMM
}

func newGroupService(jimm *jimm.JIMM) *groupsService {
	return &groupsService{
		jimm,
	}
}

// ListGroups returns a page of Group objects of at least `size` elements if available.
func (s *groupsService) ListGroups(ctx context.Context, params *resources.GetGroupsParams) (*resources.PaginatedResponse[resources.Group], error) {
	return nil, nil
}

// CreateGroup creates a single Group.
func (s *groupsService) CreateGroup(ctx context.Context, group *resources.Group) (*resources.Group, error) {
	return nil, nil
}

// GetGroup returns a single Group identified by `groupId`.
func (s *groupsService) GetGroup(ctx context.Context, groupId string) (*resources.Group, error) {
	return nil, nil
}

// UpdateGroup updates a Group.
func (s *groupsService) UpdateGroup(ctx context.Context, group *resources.Group) (*resources.Group, error) {
	return nil, nil
}

// DeleteGroup deletes a Group identified by `groupId`.
// returns (true, nil) in case the group was successfully deleted.
// returns (false, error) in case something went wrong.
// implementors may want to return (false, nil) for idempotency cases.
func (s *groupsService) DeleteGroup(ctx context.Context, groupId string) (bool, error) {
	return false, nil
}

// GetGroupIdentities returns a page of identities in a Group identified by `groupId`.
func (s *groupsService) GetGroupIdentities(ctx context.Context, groupId string, params *resources.GetGroupsItemIdentitiesParams) (*resources.PaginatedResponse[resources.Identity], error) {
	return nil, nil
}

// PatchGroupIdentities performs addition or removal of identities to/from a Group identified by `groupId`.
func (s *groupsService) PatchGroupIdentities(ctx context.Context, groupId string, identityPatches []resources.GroupIdentitiesPatchItem) (bool, error) {
	return false, nil
}

// GetGroupRoles returns a page of Roles for Group `groupId`.
func (s *groupsService) GetGroupRoles(ctx context.Context, groupId string, params *resources.GetGroupsItemRolesParams) (*resources.PaginatedResponse[resources.Role], error) {
	// TODO: I think we can remove this, JIMM doesn't have group roles.
	return nil, nil
}

// PatchGroupRoles performs addition or removal of a Role to/from a Group identified by `groupId`.
func (s *groupsService) PatchGroupRoles(ctx context.Context, groupId string, rolePatches []resources.GroupRolesPatchItem) (bool, error) {
	// TODO: I think we can remove this, JIMM doesn't have group roles.
	return false, nil
}

// GetGroupEntitlements returns a page of Entitlements for Group `groupId`.
func (s *groupsService) GetGroupEntitlements(ctx context.Context, groupId string, params *resources.GetGroupsItemEntitlementsParams) (*resources.PaginatedResponse[resources.EntityEntitlement], error) {
	return nil, nil
}

// PatchGroupEntitlements performs addition or removal of an Entitlement to/from a Group identified by `groupId`.
func (s *groupsService) PatchGroupEntitlements(ctx context.Context, groupId string, entitlementPatches []resources.GroupEntitlementsPatchItem) (bool, error) {
	return false, nil
}
