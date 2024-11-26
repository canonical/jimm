// Copyright 2024 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// RoleManager is an implementation of the jujuapi.RoleManager interface.
type RoleManager struct {
	AddRole_       func(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error)
	CountRoles_    func(ctx context.Context, user *openfga.User) (int, error)
	GetRoleByUUID_ func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error)
	GetRoleByName_ func(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error)
	ListRoles_     func(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error)
	RenameRole_    func(ctx context.Context, user *openfga.User, oldName, newName string) error
	RemoveRole_    func(ctx context.Context, user *openfga.User, name string) error
}

func (j RoleManager) AddRole(ctx context.Context, u *openfga.User, name string) (*dbmodel.RoleEntry, error) {
	if j.AddRole_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AddRole_(ctx, u, name)
}

func (j RoleManager) CountRoles(ctx context.Context, user *openfga.User) (int, error) {
	if j.CountRoles_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return j.CountRoles_(ctx, user)
}

func (j RoleManager) GetRoleByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error) {
	if j.GetRoleByUUID_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetRoleByUUID_(ctx, user, uuid)
}

func (j RoleManager) GetRoleByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error) {
	if j.GetRoleByName_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetRoleByName_(ctx, user, name)
}

func (j RoleManager) ListRoles(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error) {
	if j.ListRoles_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListRoles_(ctx, user, pagination, match)
}

func (j RoleManager) RemoveRole(ctx context.Context, user *openfga.User, name string) error {
	if j.RemoveRole_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveRole_(ctx, user, name)
}

func (j RoleManager) RenameRole(ctx context.Context, user *openfga.User, oldName, newName string) error {
	if j.RenameRole_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RenameRole_(ctx, user, oldName, newName)
}
