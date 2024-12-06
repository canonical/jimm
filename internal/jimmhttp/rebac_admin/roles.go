// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

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

// rolesService implements the `RolesService` interface.
type rolesService struct {
	jimm jujuapi.JIMM
}

func newRoleService(jimm jujuapi.JIMM) *rolesService {
	return &rolesService{
		jimm,
	}
}

// ListRoles returns a page of Role objects of at least `size` elements if available.
func (s *rolesService) ListRoles(ctx context.Context, params *resources.GetRolesParams) (*resources.PaginatedResponse[resources.Role], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.jimm.RoleManager().CountRoles(ctx, user)
	if err != nil {
		return nil, err
	}
	page, nextPage, pagination := pagination.CreatePagination(params.Size, params.Page, count)
	match := ""
	if params.Filter != nil && *params.Filter != "" {
		match = *params.Filter
	}
	roles, err := s.jimm.RoleManager().ListRoles(ctx, user, pagination, match)
	if err != nil {
		return nil, err
	}

	data := make([]resources.Role, 0, len(roles))
	for _, role := range roles {
		data = append(data, resources.Role{Id: &role.UUID, Name: role.Name})
	}
	resp := resources.PaginatedResponse[resources.Role]{
		Data: data,
		Meta: resources.ResponseMeta{
			Page:  &page,
			Size:  len(roles),
			Total: &count,
		},
		Next: resources.Next{Page: nextPage},
	}
	return &resp, nil
}

// CreateRole creates a single Role.
func (s *rolesService) CreateRole(ctx context.Context, role *resources.Role) (*resources.Role, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	roleInfo, err := s.jimm.RoleManager().AddRole(ctx, user, role.Name)
	if err != nil {
		return nil, err
	}
	return &resources.Role{Id: &roleInfo.UUID, Name: roleInfo.Name}, nil
}

// GetRole returns a single Role identified by `roleId`.
func (s *rolesService) GetRole(ctx context.Context, roleId string) (*resources.Role, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	role, err := s.jimm.RoleManager().GetRoleByUUID(ctx, user, roleId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError("failed to find role")
		}
		return nil, err
	}
	return &resources.Role{Id: &role.UUID, Name: role.Name}, nil
}

// UpdateRole updates a Role.
func (s *rolesService) UpdateRole(ctx context.Context, role *resources.Role) (*resources.Role, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if role.Id == nil {
		return nil, v1.NewValidationError("missing role ID")
	}
	existingRole, err := s.jimm.RoleManager().GetRoleByUUID(ctx, user, *role.Id)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError("failed to find role")
		}
		return nil, err
	}
	err = s.jimm.RoleManager().RenameRole(ctx, user, existingRole.Name, role.Name)
	if err != nil {
		return nil, err
	}
	return &resources.Role{Id: &existingRole.UUID, Name: role.Name}, nil
}

// DeleteRole deletes a Role identified by `roleId`.
// returns (true, nil) in case the role was successfully deleted.
// returns (false, error) in case something went wrong.
// implementors may want to return (false, nil) for idempotency cases.
func (s *rolesService) DeleteRole(ctx context.Context, roleId string) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}
	existingRole, err := s.jimm.RoleManager().GetRoleByUUID(ctx, user, roleId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	err = s.jimm.RoleManager().RemoveRole(ctx, user, existingRole.Name)
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetRoleEntitlements returns a page of Entitlements for Role `roleId`.
func (s *rolesService) GetRoleEntitlements(ctx context.Context, roleId string, params *resources.GetRolesItemEntitlementsParams) (*resources.PaginatedResponse[resources.EntityEntitlement], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	ok := jimmnames.IsValidRoleId(roleId)
	if !ok {
		return nil, v1.NewValidationError("invalid role ID")
	}
	filter := utils.CreateTokenPaginationFilter(params.Size, params.NextToken, params.NextPageToken)
	role := ofganames.WithAssigneeRelation(jimmnames.NewRoleTag(roleId))
	entitlementToken := pagination.NewEntitlementToken(filter.Token())
	// nolint:gosec accept integer conversion
	tuples, nextEntitlmentToken, err := s.jimm.ListObjectRelations(ctx, user, role, int32(filter.Limit()), entitlementToken) // #nosec G115 accept integer conversion
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

// PatchRoleEntitlements performs addition or removal of an Entitlement to/from a Role identified by `roleId`.
func (s *rolesService) PatchRoleEntitlements(ctx context.Context, roleId string, entitlementPatches []resources.RoleEntitlementsPatchItem) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}
	if !jimmnames.IsValidRoleId(roleId) {
		return false, v1.NewValidationError("invalid role ID")
	}
	roleTag := jimmnames.NewRoleTag(roleId)
	var toRemove []apiparams.RelationshipTuple
	var toAdd []apiparams.RelationshipTuple
	var errList utils.MultiErr
	toTargetTag := func(entitlementPatch resources.RoleEntitlementsPatchItem) (names.Tag, error) {
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
			Object:       ofganames.WithAssigneeRelation(roleTag),
			Relation:     entitlementPatch.Entitlement.Entitlement,
			TargetObject: tag.String(),
		}
		if entitlementPatch.Op == resources.Add {
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
