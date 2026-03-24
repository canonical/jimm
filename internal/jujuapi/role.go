// Copyright 2025 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// AddRole creates a role within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddRole(ctx context.Context, req apiparams.AddRoleRequest) (apiparams.AddRoleResponse, error) {

	resp := apiparams.AddRoleResponse{}

	if !jimmnames.IsValidRoleName(req.Name) {
		return resp, errors.E(errors.CodeBadRequest, "invalid role name")
	}

	roleEntry, err := r.jimm.RoleManager().AddRole(ctx, r.user, req.Name)
	if err != nil {
		return resp, errors.E(fmt.Errorf("failed to add role: %w", err))
	}
	resp = apiparams.AddRoleResponse{Role: apiparams.Role{
		Name:      roleEntry.Name,
		UUID:      roleEntry.UUID,
		CreatedAt: roleEntry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: roleEntry.UpdatedAt.Format(time.RFC3339),
	}}

	return resp, nil
}

// GetRole returns role information based on a UUID or name.
func (r *controllerRoot) GetRole(ctx context.Context, req apiparams.GetRoleRequest) (apiparams.Role, error) {

	var roleEntry *dbmodel.RoleEntry
	var err error
	switch {
	case req.UUID != "" && req.Name != "":
		return apiparams.Role{}, errors.E(errors.CodeBadRequest, "only one of UUID or Name should be provided")
	case req.Name != "" && !jimmnames.IsValidRoleName(req.Name):
		return apiparams.Role{}, errors.E(errors.CodeBadRequest, "invalid role name")
	case req.UUID != "":
		roleEntry, err = r.jimm.RoleManager().GetRoleByUUID(ctx, r.user, req.UUID)
	case req.Name != "":
		roleEntry, err = r.jimm.RoleManager().GetRoleByName(ctx, r.user, req.Name)
	default:
		return apiparams.Role{}, errors.E(errors.CodeBadRequest, "no UUID or Name provided")
	}
	if err != nil {
		return apiparams.Role{}, errors.E(fmt.Errorf("failed to get role: %w", err))
	}

	return apiparams.Role{
		UUID:      roleEntry.UUID,
		Name:      roleEntry.Name,
		CreatedAt: roleEntry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: roleEntry.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// RenameRole renames a role within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RenameRole(ctx context.Context, req apiparams.RenameRoleRequest) error {

	if !jimmnames.IsValidRoleName(req.NewName) {
		return errors.E(errors.CodeBadRequest, "invalid role name")
	}

	if err := r.jimm.RoleManager().RenameRole(ctx, r.user, req.Name, req.NewName); err != nil {
		return errors.E(fmt.Errorf("failed to rename role: %w", err))
	}
	return nil
}

// RemoveRole removes a role within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RemoveRole(ctx context.Context, req apiparams.RemoveRoleRequest) error {

	if !jimmnames.IsValidRoleName(req.Name) {
		return errors.E(errors.CodeBadRequest, "invalid role name")
	}

	if err := r.jimm.RoleManager().RemoveRole(ctx, r.user, req.Name); err != nil {
		return errors.E(fmt.Errorf("failed to remove role: %w", err))
	}
	return nil
}

// ListRole lists access control roles within JIMMs DB.
func (r *controllerRoot) ListRoles(ctx context.Context, req apiparams.ListRolesRequest) (apiparams.ListRoleResponse, error) {

	pagination := pagination.NewOffsetFilter(req.Limit, req.Offset)
	roles, err := r.jimm.RoleManager().ListRoles(ctx, r.user, pagination, "")
	if err != nil {
		return apiparams.ListRoleResponse{}, err
	}
	rolesResponse := make([]apiparams.Role, len(roles))
	for i, g := range roles {
		rolesResponse[i] = apiparams.Role{
			UUID:      g.UUID,
			Name:      g.Name,
			CreatedAt: g.CreatedAt.Format(time.RFC3339),
			UpdatedAt: g.UpdatedAt.Format(time.RFC3339),
		}
	}

	return apiparams.ListRoleResponse{Roles: rolesResponse}, nil
}
