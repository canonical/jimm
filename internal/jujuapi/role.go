// Copyright 2024 Canonical.

package jujuapi

import (
	"context"
	"time"

	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// AddRole creates a role within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddRole(ctx context.Context, req apiparams.AddRoleRequest) (apiparams.AddRoleResponse, error) {
	const op = errors.Op("jujuapi.AddRole")
	resp := apiparams.AddRoleResponse{}

	if !jimmnames.IsValidRoleName(req.Name) {
		return resp, errors.E(op, errors.CodeBadRequest, "invalid role name")
	}

	roleEntry, err := r.jimm.RoleManager().AddRole(ctx, r.user, req.Name)
	if err != nil {
		zapctx.Error(ctx, "failed to add role", zaputil.Error(err))
		return resp, errors.E(op, err)
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
	const op = errors.Op("jujuapi.GetRole")

	var roleEntry *dbmodel.RoleEntry
	var err error
	switch {
	case req.UUID != "" && req.Name != "":
		return apiparams.Role{}, errors.E(op, errors.CodeBadRequest, "only one of UUID or Name should be provided")
	case req.Name != "" && !jimmnames.IsValidRoleName(req.Name):
		return apiparams.Role{}, errors.E(op, errors.CodeBadRequest, "invalid role name")
	case req.UUID != "":
		roleEntry, err = r.jimm.RoleManager().GetRoleByUUID(ctx, r.user, req.UUID)
	case req.Name != "":
		roleEntry, err = r.jimm.RoleManager().GetRoleByName(ctx, r.user, req.Name)
	default:
		return apiparams.Role{}, errors.E(op, errors.CodeBadRequest, "no UUID or Name provided")
	}
	if err != nil {
		zapctx.Error(ctx, "failed to get role", zaputil.Error(err))
		return apiparams.Role{}, errors.E(op, err)
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
	const op = errors.Op("jujuapi.RenameRole")

	if !jimmnames.IsValidRoleName(req.NewName) {
		return errors.E(op, errors.CodeBadRequest, "invalid role name")
	}

	if err := r.jimm.RoleManager().RenameRole(ctx, r.user, req.Name, req.NewName); err != nil {
		zapctx.Error(ctx, "failed to rename role", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// RemoveRole removes a role within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RemoveRole(ctx context.Context, req apiparams.RemoveRoleRequest) error {
	const op = errors.Op("jujuapi.RemoveRole")

	if !jimmnames.IsValidRoleName(req.Name) {
		return errors.E(op, errors.CodeBadRequest, "invalid role name")
	}

	if err := r.jimm.RoleManager().RemoveRole(ctx, r.user, req.Name); err != nil {
		zapctx.Error(ctx, "failed to remove role", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// ListRole lists access control roles within JIMMs DB.
func (r *controllerRoot) ListRoles(ctx context.Context, req apiparams.ListRolesRequest) (apiparams.ListRoleResponse, error) {
	const op = errors.Op("jujuapi.ListRoles")

	pagination := pagination.NewOffsetFilter(req.Limit, req.Offset)
	roles, err := r.jimm.RoleManager().ListRoles(ctx, r.user, pagination, "")
	if err != nil {
		return apiparams.ListRoleResponse{}, errors.E(op, err)
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
