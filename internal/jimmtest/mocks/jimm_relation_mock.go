// Copyright 2024 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// RelationService is an implementation of the jujuapi.RelationService interface.
type RelationService struct {
	AddRelation_            func(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	RemoveRelation_         func(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	CheckRelation_          func(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error)
	ListRelationshipTuples_ func(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
	ListObjectRelations_    func(ctx context.Context, user *openfga.User, object string, pageSize int32, continuationToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error)
}

func (j *RelationService) AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	if j.AddRelation_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddRelation_(ctx, user, tuples)
}

func (j *RelationService) RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	if j.RemoveRelation_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveRelation_(ctx, user, tuples)
}

func (j *RelationService) CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error) {
	if j.CheckRelation_ == nil {
		return false, errors.E(errors.CodeNotImplemented)
	}
	return j.CheckRelation_(ctx, user, tuple, trace)
}

func (j *RelationService) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {
	if j.ListRelationshipTuples_ == nil {
		return []openfga.Tuple{}, "", errors.E(errors.CodeNotImplemented)
	}
	return j.ListRelationshipTuples_(ctx, user, tuple, pageSize, continuationToken)
}

func (j *RelationService) ListObjectRelations(ctx context.Context, user *openfga.User, object string, pageSize int32, entitlementToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error) {
	if j.ListObjectRelations_ == nil {
		return []openfga.Tuple{}, pagination.EntitlementToken{}, errors.E(errors.CodeNotImplemented)
	}
	return j.ListObjectRelations_(ctx, user, object, pageSize, entitlementToken)
}
