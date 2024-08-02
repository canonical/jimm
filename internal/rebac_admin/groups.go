// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"context"

	v1 "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/rebac_admin/utils"
)

// groupsService implements the `GroupsService` interface.
type groupsService struct {
	jimm jujuapi.JIMM
}

func newGroupService(jimm jujuapi.JIMM) *groupsService {
	return &groupsService{
		jimm,
	}
}

// ListGroups returns a page of Group objects of at least `size` elements if available.
func (s *groupsService) ListGroups(ctx context.Context, params *resources.GetGroupsParams) (*resources.PaginatedResponse[resources.Group], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	currentPage, filter := utils.CreatePaginationFilter(params.Size, params.Page)
	nextPage := currentPage + 1
	count, err := s.jimm.CountGroups(ctx, user)
	if err != nil {
		return nil, err
	}
	groups, err := s.jimm.ListGroups(ctx, user, filter)
	if err != nil {
		return nil, err
	}
	data := make([]resources.Group, 0, len(groups))
	for _, group := range groups {
		data = append(data, resources.Group{Id: &group.UUID, Name: group.Name})
	}
	resp := resources.PaginatedResponse[resources.Group]{
		Data: data,
		Meta: resources.ResponseMeta{
			Page:  &currentPage,
			Size:  len(groups),
			Total: &count,
		},
		Next: resources.Next{Page: &nextPage},
	}
	return &resp, nil
}

// CreateGroup creates a single Group.
func (s *groupsService) CreateGroup(ctx context.Context, group *resources.Group) (*resources.Group, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	groupInfo, err := s.jimm.AddGroup(ctx, user, group.Name)
	if err != nil {
		return nil, err
	}
	return &resources.Group{Id: &groupInfo.UUID, Name: groupInfo.Name}, nil
}

// GetGroup returns a single Group identified by `groupId`.
func (s *groupsService) GetGroup(ctx context.Context, groupId string) (*resources.Group, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	group, err := s.jimm.GetGroupByID(ctx, user, groupId)
	if err != nil {
		return nil, err
	}
	return &resources.Group{Id: &group.UUID, Name: group.Name}, nil
}

// UpdateGroup updates a Group.
func (s *groupsService) UpdateGroup(ctx context.Context, group *resources.Group) (*resources.Group, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if group.Id == nil {
		return nil, v1.NewValidationError("missing group ID")
	}
	existingGroup, err := s.jimm.GetGroupByID(ctx, user, *group.Id)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError("failed to find group")
		}
		return nil, err
	}
	err = s.jimm.RenameGroup(ctx, user, existingGroup.Name, group.Name)
	if err != nil {
		return nil, err
	}
	return &resources.Group{Id: &existingGroup.UUID, Name: group.Name}, nil
}

// DeleteGroup deletes a Group identified by `groupId`.
// returns (true, nil) in case the group was successfully deleted.
// returns (false, error) in case something went wrong.
// implementors may want to return (false, nil) for idempotency cases.
func (s *groupsService) DeleteGroup(ctx context.Context, groupId string) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}
	existingGroup, err := s.jimm.GetGroupByID(ctx, user, groupId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	err = s.jimm.RemoveGroup(ctx, user, existingGroup.Name)
	if err != nil {
		return false, err
	}
	return true, nil
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
