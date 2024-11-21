// Copyright 2024 Canonical.
package role

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// RoleStore defines a storage interface to persist roles.
type RoleStore interface {
	// AddRole adds a new role.
	AddRole(ctx context.Context, name string) (re *dbmodel.RoleEntry, err error)

	// GetRole populates the provided *dbmodel.RoleEntry based on name or UUID.
	GetRole(ctx context.Context, role *dbmodel.RoleEntry) (err error)

	// UpdateRoleName updates the name of a role identified by UUID.
	UpdateRoleName(ctx context.Context, uuid, name string) (err error)

	// RemoveRole removes the role identified by its ID or UUID.
	RemoveRole(ctx context.Context, role *dbmodel.RoleEntry) (err error)

	// ListRoles returns a paginated list of Roles defined by limit and offset.
	// match is used to fuzzy find based on entries' name or uuid using the LIKE operator (ex. LIKE %<match>%).
	ListRoles(ctx context.Context, limit, offset int, match string) (_ []dbmodel.RoleEntry, err error)

	// CountRoles returns a count of the number of Roles that exist.
	CountRoles(ctx context.Context) (count int, err error)
}

// RoleAuthorisationService holds the interface to manage roles within an authorisation service.
type RoleAuthorisationService interface {
	// RemoveRole removes a role.
	RemoveRole(ctx context.Context, role jimmnames.RoleTag) error
}

// roleManager provides a means to manage roles within JIMM.
type roleManager struct {
	store   RoleStore
	authSvc RoleAuthorisationService
}

// NewRoleManager returns a new RoleManager that persists the roles in the provided store.
func NewRoleManager(store RoleStore, authSvc RoleAuthorisationService) (*roleManager, error) {
	if store == nil {
		return nil, errors.E("role store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("role authorisation service cannot be nil")
	}
	return &roleManager{store, authSvc}, nil
}

// AddRole adds a role to JIMM.
func (rm *roleManager) AddRole(ctx context.Context, user *openfga.User, roleName string) (*dbmodel.RoleEntry, error) {
	const op = errors.Op("jimm.AddRole")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	re, err := rm.store.AddRole(ctx, roleName)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return re, nil
}

// GetRoleByUUID returns a role based on the provided UUID.
func (rm *roleManager) GetRoleByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error) {
	return rm.getRole(ctx, user, &dbmodel.RoleEntry{UUID: uuid})
}

// GetRoleByName returns a role based on the provided name.
func (rm *roleManager) GetRoleByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error) {
	return rm.getRole(ctx, user, &dbmodel.RoleEntry{Name: name})
}

// RemoveRole removes the role from JIMM in both the store and authorisation store.
func (rm *roleManager) RemoveRole(ctx context.Context, user *openfga.User, roleName string) error {
	const op = errors.Op("jimm.RemoveRole")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	re := &dbmodel.RoleEntry{
		Name: roleName,
	}
	err := rm.store.GetRole(ctx, re)
	if err != nil {
		return errors.E(op, err)
	}

	// TODO(ale8k):
	// Would be nice to have a way to create a transaction to get, remove tuples, if successful, delete role
	// somehow. We could pass a callback and change the db methods?
	if err := rm.authSvc.RemoveRole(ctx, re.ResourceTag()); err != nil {
		return errors.E(op, err)
	}

	if err := rm.store.RemoveRole(ctx, re); err != nil {
		return errors.E(op, err)
	}

	return nil
}

// RenameRole renames a role in JIMM's DB.
func (rm *roleManager) RenameRole(ctx context.Context, user *openfga.User, uuid, newName string) error {
	const op = errors.Op("jimm.RenameRole")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	err := rm.store.UpdateRoleName(ctx, uuid, newName)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// ListRoles returns a list of roles known to JIMM.
// `match` will filter the list fuzzy matching role's name or uuid.
func (rm *roleManager) ListRoles(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error) {
	const op = errors.Op("jimm.ListRoles")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	res, err := rm.store.ListRoles(ctx, pagination.Limit(), pagination.Offset(), match)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}

// CountRoles returns the number of roles that exist.
func (rm *roleManager) CountRoles(ctx context.Context, user *openfga.User) (int, error) {
	const op = errors.Op("jimm.CountRoles")

	if !user.JimmAdmin {
		return 0, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	count, err := rm.store.CountRoles(ctx)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return count, nil
}

// getRole returns a role based on the provided UUID or name.
func (rm *roleManager) getRole(ctx context.Context, user *openfga.User, role *dbmodel.RoleEntry) (*dbmodel.RoleEntry, error) {
	const op = errors.Op("jimm.getRole")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	if err := rm.store.GetRole(ctx, role); err != nil {
		return nil, errors.E(op, err)
	}

	return role, nil
}
