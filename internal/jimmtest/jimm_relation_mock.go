// Copyright 2024 Canonical Ltd.

package jimmtest

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// Below, every struct is a mock of a different JIMM service. Every method
// has a corresponding function field. Whenever the method is called it
// will delegate to the requested funcion or if the funcion is nil return
// a NotImplemented error.

// GroupService is an implementation of the jujuapi.GroupService interface.
type GroupService struct {
	AddGroup_     func(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error)
	CountGroups_  func(ctx context.Context, user *openfga.User) (int, error)
	GetGroupByID_ func(ctx context.Context, user *openfga.User, uuid string) (dbmodel.GroupEntry, error)
	ListGroups_   func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error)
	RenameGroup_  func(ctx context.Context, user *openfga.User, oldName, newName string) error
	RemoveGroup_  func(ctx context.Context, user *openfga.User, name string) error
}

func (j *JIMM) AddGroup(ctx context.Context, u *openfga.User, name string) (*dbmodel.GroupEntry, error) {
	if j.AddGroup_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AddGroup_(ctx, u, name)
}

func (j *JIMM) CountGroups(ctx context.Context, user *openfga.User) (int, error) {
	if j.CountGroups_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return j.CountGroups_(ctx, user)
}

func (j *JIMM) GetGroupByID(ctx context.Context, user *openfga.User, uuid string) (dbmodel.GroupEntry, error) {
	if j.GetGroupByID_ == nil {
		return dbmodel.GroupEntry{}, errors.E(errors.CodeNotImplemented)
	}
	return j.GetGroupByID_(ctx, user, uuid)
}

func (j *JIMM) ListGroups(ctx context.Context, user *openfga.User, filters pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error) {
	if j.ListGroups_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListGroups_(ctx, user, filters)
}

func (j *JIMM) RemoveGroup(ctx context.Context, user *openfga.User, name string) error {
	if j.RemoveGroup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveGroup_(ctx, user, name)
}

func (j *JIMM) RenameGroup(ctx context.Context, user *openfga.User, oldName, newName string) error {
	if j.RenameGroup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RenameGroup_(ctx, user, oldName, newName)
}

// RelationService is an implementation of the jujuapi.RelationService interface.
type RelationService struct {
	AddRelation_            func(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	RemoveRelation_         func(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	CheckRelation_          func(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error)
	ListRelationshipTuples_ func(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
}

func (j *JIMM) AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	if j.AddRelation_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddRelation_(ctx, user, tuples)
}

func (j *JIMM) RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	if j.RemoveRelation_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveRelation_(ctx, user, tuples)
}

func (j *JIMM) CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error) {
	if j.CheckRelation_ == nil {
		return false, errors.E(errors.CodeNotImplemented)
	}
	return j.CheckRelation_(ctx, user, tuple, trace)
}

func (j *JIMM) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {
	if j.ListRelationshipTuples_ == nil {
		return []openfga.Tuple{}, "", errors.E(errors.CodeNotImplemented)
	}
	return j.ListRelationshipTuples_(ctx, user, tuple, pageSize, continuationToken)
}
