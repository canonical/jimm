// Copyright 2024 Canonical.
package jimm

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// AddRole creates a role within JIMMs DB for reference by OpenFGA.
func (j *JIMM) AddRole(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error) {
	return &dbmodel.RoleEntry{}, nil
}

// CountRoles returns the number of roles that exist.
func (j *JIMM) CountRoles(ctx context.Context, user *openfga.User) (int, error) {
	return 0, nil
}

// getRole returns a role based on the provided UUID or name.
func (j *JIMM) getRole(_ context.Context, _ *openfga.User, _ *dbmodel.RoleEntry) (*dbmodel.RoleEntry, error) {
	return &dbmodel.RoleEntry{}, nil
}

// GetRoleByUUID returns a role based on the provided UUID.
func (j *JIMM) GetRoleByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error) {
	return j.getRole(ctx, user, &dbmodel.RoleEntry{UUID: uuid})
}

// GetRoleByName returns a role based on the provided name.
func (j *JIMM) GetRoleByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error) {
	return j.getRole(ctx, user, &dbmodel.RoleEntry{Name: name})
}

// RenameRole renames a role in JIMM's DB.
func (j *JIMM) RenameRole(ctx context.Context, user *openfga.User, oldName, newName string) error {
	return nil
}

// RemoveRole removes a role within JIMMs DB for reference by OpenFGA.
func (j *JIMM) RemoveRole(ctx context.Context, user *openfga.User, name string) error {
	return nil
}

// ListRoles returns a list of roles known to JIMM.
// `match` will filter the list fuzzy matching role's name or uuid.
func (j *JIMM) ListRoles(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error) {
	return []dbmodel.RoleEntry{}, nil
}
