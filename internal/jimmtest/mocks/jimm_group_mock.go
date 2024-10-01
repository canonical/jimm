// Copyright 2024 Canonical.

// This package contains mocks for each JIMM service.
// Each file contains a struct providing tests with the ability to mock
// JIMM services on test-by-test basis. Each struct has a corresponding
// function field. Whenever the method is called it will delegate to the
// requested funcion or if the funcion is nil return a NotImplemented error.
package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// GroupService is an implementation of the jujuapi.GroupService interface.
type GroupService struct {
	AddGroup_       func(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error)
	CountGroups_    func(ctx context.Context, user *openfga.User) (int, error)
	GetGroupByUUID_ func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error)
	GetGroupByName_ func(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error)
	ListGroups_     func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error)
	RenameGroup_    func(ctx context.Context, user *openfga.User, oldName, newName string) error
	RemoveGroup_    func(ctx context.Context, user *openfga.User, name string) error
}

func (j *GroupService) AddGroup(ctx context.Context, u *openfga.User, name string) (*dbmodel.GroupEntry, error) {
	if j.AddGroup_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AddGroup_(ctx, u, name)
}

func (j *GroupService) CountGroups(ctx context.Context, user *openfga.User) (int, error) {
	if j.CountGroups_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return j.CountGroups_(ctx, user)
}

func (j *GroupService) GetGroupByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error) {
	if j.GetGroupByUUID_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetGroupByUUID_(ctx, user, uuid)
}

func (j *GroupService) GetGroupByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error) {
	if j.GetGroupByName_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetGroupByName_(ctx, user, name)
}

func (j *GroupService) ListGroups(ctx context.Context, user *openfga.User, filters pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error) {
	if j.ListGroups_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListGroups_(ctx, user, filters)
}

func (j *GroupService) RemoveGroup(ctx context.Context, user *openfga.User, name string) error {
	if j.RemoveGroup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveGroup_(ctx, user, name)
}

func (j *GroupService) RenameGroup(ctx context.Context, user *openfga.User, oldName, newName string) error {
	if j.RenameGroup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RenameGroup_(ctx, user, oldName, newName)
}
