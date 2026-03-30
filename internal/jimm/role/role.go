// Copyright 2025 Canonical.

package role

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// RoleManager provides a means to manage roles within JIMM.
type RoleManager struct {
	store   *db.Database
	authSvc *openfga.OFGAClient
}

// NewRoleManager returns a new RoleManager that persists the roles in the provided store.
func NewRoleManager(store *db.Database, authSvc *openfga.OFGAClient) (*RoleManager, error) {
	if store == nil {
		return nil, errors.New("role store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("role authorisation service cannot be nil")
	}
	return &RoleManager{store, authSvc}, nil
}

// AddRole adds a role to JIMM.
func (rm *RoleManager) AddRole(ctx context.Context, user *openfga.User, roleName string) (*dbmodel.RoleEntry, error) {
	if !user.JimmAdmin {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	re, err := rm.store.AddRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	return re, nil
}

// GetRoleByUUID returns a role based on the provided UUID.
func (rm *RoleManager) GetRoleByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error) {
	return rm.getRole(ctx, user, &dbmodel.RoleEntry{UUID: uuid})
}

// GetRoleByName returns a role based on the provided name.
func (rm *RoleManager) GetRoleByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error) {
	return rm.getRole(ctx, user, &dbmodel.RoleEntry{Name: name})
}

// RemoveRole removes the role from JIMM in both the store and authorisation store.
func (rm *RoleManager) RemoveRole(ctx context.Context, user *openfga.User, roleName string) error {
	if !user.JimmAdmin {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	re := &dbmodel.RoleEntry{
		Name: roleName,
	}
	err := rm.store.GetRole(ctx, re)
	if err != nil {
		return err
	}

	// TODO(ale8k):
	// Would be nice to have a way to create a transaction to get, remove tuples, if successful, delete role
	// somehow. We could pass a callback and change the db methods?
	if err := rm.authSvc.RemoveRole(ctx, re.ResourceTag()); err != nil {
		return err
	}

	if err := rm.store.RemoveRole(ctx, re); err != nil {
		return err
	}

	return nil
}

// RenameRole renames a role in JIMM's DB.
func (rm *RoleManager) RenameRole(ctx context.Context, user *openfga.User, oldName, newName string) error {
	if !user.JimmAdmin {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	err := rm.store.UpdateRoleName(ctx, oldName, newName)
	if err != nil {
		return err
	}

	return nil
}

// ListRoles returns a list of roles known to JIMM.
// `match` will filter the list fuzzy matching role's name or uuid.
func (rm *RoleManager) ListRoles(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error) {
	if !user.JimmAdmin {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	res, err := rm.store.ListRoles(ctx, pagination.Limit(), pagination.Offset(), match)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// CountRoles returns the number of roles that exist.
func (rm *RoleManager) CountRoles(ctx context.Context, user *openfga.User) (int, error) {
	if !user.JimmAdmin {
		return 0, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	count, err := rm.store.CountRoles(ctx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// getRole returns a role based on the provided UUID or name.
func (rm *RoleManager) getRole(ctx context.Context, user *openfga.User, role *dbmodel.RoleEntry) (*dbmodel.RoleEntry, error) {
	if !user.JimmAdmin {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	if err := rm.store.GetRole(ctx, role); err != nil {
		return nil, err
	}

	return role, nil
}
