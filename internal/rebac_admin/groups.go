// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"context"

	v1 "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/interfaces"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// GroupsService implements the `GroupsService` interface.
type GroupsService struct {
	Database *database.Database
}

// For doc/test sake, to hint that the struct needs to implement a specific interface.
var _ interfaces.GroupsService = &GroupsService{}

// ListGroups returns a page of Group objects of at least `size` elements if available.
func (s *GroupsService) ListGroups(ctx context.Context, params *resources.GetGroupsParams) (*resources.PaginatedResponse[resources.Group], error) {
	// For the sake of this example we allow everyone to call this method. If it's not
	// the case, you can do the following to get the user:
	//
	//    raw, _ := v1.GetIdentityFromContext(ctx)
	//    user, _ := raw.(*User)
	//
	// And return this error if the user is not authorized:
	//
	//    return nil, v1.NewAuthorizationError("user cannot add group")
	//

	return Paginate(s.Database.ListGroups(), params.Size, params.Page, params.NextToken, params.NextPageToken, false)
}

// CreateGroup creates a single Group.
func (s *GroupsService) CreateGroup(ctx context.Context, group *resources.Group) (*resources.Group, error) {
	// For the sake of this example we allow everyone to call this method.

	added, err := s.Database.AddGroup(group)
	if err != nil {
		return nil, v1.NewInvalidRequestError("already exists")
	}
	return added, nil
}

// GetGroup returns a single Group identified by `groupId`.
func (s *GroupsService) GetGroup(ctx context.Context, groupId string) (*resources.Group, error) {
	// For the sake of this example we allow everyone to call this method.

	group := s.Database.GetGroup(groupId)
	if group == nil {
		return nil, v1.NewNotFoundError("")
	}
	return group, nil
}

// UpdateGroup updates a Group.
func (s *GroupsService) UpdateGroup(ctx context.Context, group *resources.Group) (*resources.Group, error) {
	// For the sake of this example we allow everyone to call this method.

	updated := s.Database.UpdateGroup(ctx, group)
	if updated == nil {
		return nil, v1.NewNotFoundError("")
	}
	return updated, nil
}

// DeleteGroup deletes a Group identified by `groupId`.
// returns (true, nil) in case the group was successfully deleted.
// returns (false, error) in case something went wrong.
// implementors may want to return (false, nil) for idempotency cases.
func (s *GroupsService) DeleteGroup(ctx context.Context, groupId string) (bool, error) {
	// For the sake of this example we allow everyone to call this method.

	deleted := s.Database.DeleteGroup(groupId)

	if !deleted {
		// For idempotency, we return a nil error; the `false` value indicates
		// that the entry was already deleted/missing.
		return false, nil
	}
	return true, nil
}

// GetGroupIdentities returns a page of identities in a Group identified by `groupId`.
func (s *GroupsService) GetGroupIdentities(ctx context.Context, groupId string, params *resources.GetGroupsItemIdentitiesParams) (*resources.PaginatedResponse[resources.Identity], error) {
	// For the sake of this example we allow everyone to call this method.

	relatives := s.Database.GetGroupIdentities(groupId)
	if relatives == nil {
		return nil, v1.NewNotFoundError("")
	}
	return Paginate(relatives, params.Size, params.Page, params.NextToken, params.NextPageToken, false)
}

// PatchGroupIdentities performs addition or removal of identities to/from a Group identified by `groupId`.
func (s *GroupsService) PatchGroupIdentities(ctx context.Context, groupId string, identityPatches []resources.GroupIdentitiesPatchItem) (bool, error) {
	// For the sake of this example we allow everyone to call this method.

	additions := []string{}
	removals := []string{}
	for _, p := range identityPatches {
		if p.Op == "add" {
			additions = append(additions, p.Identity)
		} else if p.Op == "remove" {
			removals = append(removals, p.Identity)
		}
	}

	changed := s.Database.PatchGroupIdentities(groupId, additions, removals)
	if changed == nil {
		return false, v1.NewNotFoundError("")
	}
	return *changed, nil
}

// GetGroupRoles returns a page of Roles for Group `groupId`.
func (s *GroupsService) GetGroupRoles(ctx context.Context, groupId string, params *resources.GetGroupsItemRolesParams) (*resources.PaginatedResponse[resources.Role], error) {
	// For the sake of this example we allow everyone to call this method.

	relatives := s.Database.GetGroupRoles(groupId)
	if relatives == nil {
		return nil, v1.NewNotFoundError("")
	}
	return Paginate(relatives, params.Size, params.Page, params.NextToken, params.NextPageToken, false)
}

// PatchGroupRoles performs addition or removal of a Role to/from a Group identified by `groupId`.
func (s *GroupsService) PatchGroupRoles(ctx context.Context, groupId string, rolePatches []resources.GroupRolesPatchItem) (bool, error) {
	// For the sake of this example we allow everyone to call this method.

	additions := []string{}
	removals := []string{}
	for _, p := range rolePatches {
		if p.Op == "add" {
			additions = append(additions, p.Role)
		} else if p.Op == "remove" {
			removals = append(removals, p.Role)
		}
	}

	changed := s.Database.PatchGroupRoles(groupId, additions, removals)
	if changed == nil {
		return false, v1.NewNotFoundError("")
	}
	return *changed, nil
}

// GetGroupEntitlements returns a page of Entitlements for Group `groupId`.
func (s *GroupsService) GetGroupEntitlements(ctx context.Context, groupId string, params *resources.GetGroupsItemEntitlementsParams) (*resources.PaginatedResponse[resources.EntityEntitlement], error) {
	// For the sake of this example we allow everyone to call this method.

	relatives := s.Database.GetGroupEntitlements(groupId)
	if relatives == nil {
		return nil, v1.NewNotFoundError("")
	}
	return Paginate(relatives, params.Size, params.Page, params.NextToken, params.NextPageToken, false)
}

// PatchGroupEntitlements performs addition or removal of an Entitlement to/from a Group identified by `groupId`.
func (s *GroupsService) PatchGroupEntitlements(ctx context.Context, groupId string, entitlementPatches []resources.GroupEntitlementsPatchItem) (bool, error) {
	// For the sake of this example we allow everyone to call this method.

	additions := []string{}
	removals := []string{}
	for _, p := range entitlementPatches {
		if p.Op == "add" {
			additions = append(additions, database.EntitlementToString(p.Entitlement))
		} else if p.Op == "remove" {
			removals = append(removals, database.EntitlementToString(p.Entitlement))
		}
	}

	changed := s.Database.PatchGroupEntitlements(groupId, additions, removals)
	if changed == nil {
		return false, v1.NewNotFoundError("")
	}
	return *changed, nil
}
