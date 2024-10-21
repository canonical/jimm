// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"
	"fmt"

	v1 "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin/utils"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
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
	count, err := s.jimm.CountGroups(ctx, user)
	if err != nil {
		return nil, err
	}
	page, nextPage, pagination := pagination.CreatePagination(params.Size, params.Page, count)
	groups, err := s.jimm.ListGroups(ctx, user, pagination)
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
			Page:  &page,
			Size:  len(groups),
			Total: &count,
		},
		Next: resources.Next{Page: nextPage},
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
	group, err := s.jimm.GetGroupByUUID(ctx, user, groupId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError("failed to find group")
		}
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
	existingGroup, err := s.jimm.GetGroupByUUID(ctx, user, *group.Id)
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
	existingGroup, err := s.jimm.GetGroupByUUID(ctx, user, groupId)
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
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if !jimmnames.IsValidGroupId(groupId) {
		return nil, v1.NewValidationError("invalid group ID")
	}
	filter := utils.CreateTokenPaginationFilter(params.Size, params.NextToken, params.NextPageToken)
	groupTag := jimmnames.NewGroupTag(groupId)
	_, err = s.jimm.GetGroupByUUID(ctx, user, groupId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError("group not found")
		}
		return nil, err
	}
	tuple := apiparams.RelationshipTuple{
		Relation:     ofganames.MemberRelation.String(),
		TargetObject: groupTag.String(),
	}
	identities, nextToken, err := s.jimm.ListRelationshipTuples(ctx, user, tuple, int32(filter.Limit()), filter.Token()) // #nosec G115 accept integer conversion
	if err != nil {
		return nil, err
	}
	data := make([]resources.Identity, 0, len(identities))
	for _, identity := range identities {
		identifier := identity.Object.ID
		data = append(data, resources.Identity{Email: identifier})
	}
	originalToken := filter.Token()
	resp := resources.PaginatedResponse[resources.Identity]{
		Meta: resources.ResponseMeta{
			Size:      len(data),
			PageToken: &originalToken,
		},
		Data: data,
	}
	if nextToken != "" {
		resp.Next = resources.Next{
			PageToken: &nextToken,
		}
	}
	return &resp, nil
}

// PatchGroupIdentities performs addition or removal of identities to/from a Group identified by `groupId`.
func (s *groupsService) PatchGroupIdentities(ctx context.Context, groupId string, identityPatches []resources.GroupIdentitiesPatchItem) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}
	if !jimmnames.IsValidGroupId(groupId) {
		return false, v1.NewValidationError("invalid group ID")
	}
	groupTag := jimmnames.NewGroupTag(groupId)
	tuple := apiparams.RelationshipTuple{
		Relation:     ofganames.MemberRelation.String(),
		TargetObject: groupTag.String(),
	}
	var toRemove []apiparams.RelationshipTuple
	var toAdd []apiparams.RelationshipTuple
	for _, identityPatch := range identityPatches {
		if !names.IsValidUser(identityPatch.Identity) {
			return false, v1.NewValidationError(fmt.Sprintf("invalid identity: %s", identityPatch.Identity))
		}
		identity := names.NewUserTag(identityPatch.Identity)
		if identityPatch.Op == resources.GroupIdentitiesPatchItemOpAdd {
			t := tuple
			t.Object = identity.String()
			toAdd = append(toAdd, t)
		} else {
			t := tuple
			t.Object = identity.String()
			toRemove = append(toRemove, t)
		}
	}
	if toAdd != nil {
		err := s.jimm.AddRelation(ctx, user, toAdd)
		if err != nil {
			return false, err
		}
	}
	if toRemove != nil {
		err := s.jimm.RemoveRelation(ctx, user, toRemove)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// GetGroupRoles returns a page of Roles for Group `groupId`.
func (s *groupsService) GetGroupRoles(ctx context.Context, groupId string, params *resources.GetGroupsItemRolesParams) (*resources.PaginatedResponse[resources.Role], error) {
	return nil, v1.NewNotImplementedError("get group roles not implemented")
}

// PatchGroupRoles performs addition or removal of a Role to/from a Group identified by `groupId`.
func (s *groupsService) PatchGroupRoles(ctx context.Context, groupId string, rolePatches []resources.GroupRolesPatchItem) (bool, error) {
	return false, v1.NewNotImplementedError("patch group roles not implemented")
}

// GetGroupEntitlements returns a page of Entitlements for Group `groupId`.
func (s *groupsService) GetGroupEntitlements(ctx context.Context, groupId string, params *resources.GetGroupsItemEntitlementsParams) (*resources.PaginatedResponse[resources.EntityEntitlement], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	ok := jimmnames.IsValidGroupId(groupId)
	if !ok {
		return nil, v1.NewValidationError("invalid group ID")
	}
	filter := utils.CreateTokenPaginationFilter(params.Size, params.NextToken, params.NextPageToken)
	group := ofganames.WithMemberRelation(jimmnames.NewGroupTag(groupId))
	entitlementToken := pagination.NewEntitlementToken(filter.Token())
	// nolint:gosec accept integer conversion
	tuples, nextEntitlmentToken, err := s.jimm.ListObjectRelations(ctx, user, group, int32(filter.Limit()), entitlementToken) // #nosec G115 accept integer conversion
	if err != nil {
		return nil, err
	}
	originalToken := filter.Token()
	resp := resources.PaginatedResponse[resources.EntityEntitlement]{
		Meta: resources.ResponseMeta{
			Size:      len(tuples),
			PageToken: &originalToken,
		},
		Data: utils.ToEntityEntitlements(tuples),
	}
	if nextEntitlmentToken.String() != "" {
		nextToken := nextEntitlmentToken.String()
		resp.Next = resources.Next{
			PageToken: &nextToken,
		}
	}
	return &resp, nil
}

// PatchGroupEntitlements performs addition or removal of an Entitlement to/from a Group identified by `groupId`.
func (s *groupsService) PatchGroupEntitlements(ctx context.Context, groupId string, entitlementPatches []resources.GroupEntitlementsPatchItem) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}
	if !jimmnames.IsValidGroupId(groupId) {
		return false, v1.NewValidationError("invalid group ID")
	}
	groupTag := jimmnames.NewGroupTag(groupId)
	var toRemove []apiparams.RelationshipTuple
	var toAdd []apiparams.RelationshipTuple
	var errList utils.MultiErr
	toTargetTag := func(entitlementPatch resources.GroupEntitlementsPatchItem) (names.Tag, error) {
		return utils.ValidateDecomposedTag(
			entitlementPatch.Entitlement.EntityType,
			entitlementPatch.Entitlement.EntityId,
		)
	}
	for _, entitlementPatch := range entitlementPatches {
		tag, err := toTargetTag(entitlementPatch)
		if err != nil {
			errList.AppendError(err)
			continue
		}
		t := apiparams.RelationshipTuple{
			Object:       ofganames.WithMemberRelation(groupTag),
			Relation:     entitlementPatch.Entitlement.Entitlement,
			TargetObject: tag.String(),
		}
		if entitlementPatch.Op == resources.GroupEntitlementsPatchItemOpAdd {
			toAdd = append(toAdd, t)
		} else {
			toRemove = append(toRemove, t)
		}
	}
	if err := errList.Error(); err != nil {
		return false, err
	}
	if toAdd != nil {
		err := s.jimm.AddRelation(ctx, user, toAdd)
		if err != nil {
			return false, err
		}
	}
	if toRemove != nil {
		err := s.jimm.RemoveRelation(ctx, user, toRemove)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}
