// Copyright 2025 Canonical.

package mocks

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// PermissionManager is an implementation of the jujuapi.PermissionManager interface.
type PermissionManager struct {
	AddRelation_            func(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	RemoveRelation_         func(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	CheckRelation_          func(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error)
	ListRelationshipTuples_ func(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
	ListObjectRelations_    func(ctx context.Context, user *openfga.User, object string, pageSize int32, continuationToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error)

	GetJimmControllerAccess_   func(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error)
	GetUserCloudAccess_        func(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error)
	GetUserControllerAccess_   func(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error)
	GetUserModelAccess_        func(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error)
	GrantAuditLogAccess_       func(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	GrantCloudAccess_          func(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	GrantModelAccess_          func(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	GrantOfferAccess_          func(ctx context.Context, u *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error
	GrantServiceAccountAccess_ func(ctx context.Context, u *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error

	RevokeAuditLogAccess_  func(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	RevokeCloudAccess_     func(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	RevokeCloudCredential_ func(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error
	RevokeModelAccess_     func(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	RevokeOfferAccess_     func(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error)

	OpenFGACleanup_ func(ctx context.Context) error
	ToJAASTag_      func(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error)
}

func (j *PermissionManager) AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	if j.AddRelation_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddRelation_(ctx, user, tuples)
}

func (j *PermissionManager) RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	if j.RemoveRelation_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveRelation_(ctx, user, tuples)
}

func (j *PermissionManager) CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error) {
	if j.CheckRelation_ == nil {
		return false, errors.E(errors.CodeNotImplemented)
	}
	return j.CheckRelation_(ctx, user, tuple, trace)
}

func (j *PermissionManager) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {
	if j.ListRelationshipTuples_ == nil {
		return []openfga.Tuple{}, "", errors.E(errors.CodeNotImplemented)
	}
	return j.ListRelationshipTuples_(ctx, user, tuple, pageSize, continuationToken)
}

func (j *PermissionManager) ListObjectRelations(ctx context.Context, user *openfga.User, object string, pageSize int32, entitlementToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error) {
	if j.ListObjectRelations_ == nil {
		return []openfga.Tuple{}, pagination.EntitlementToken{}, errors.E(errors.CodeNotImplemented)
	}
	return j.ListObjectRelations_(ctx, user, object, pageSize, entitlementToken)
}

func (j *PermissionManager) GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error) {
	if j.GetJimmControllerAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetJimmControllerAccess_(ctx, user, tag)
}

func (j *PermissionManager) GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error) {
	if j.GetUserCloudAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetUserCloudAccess_(ctx, user, cloud)
}

func (j *PermissionManager) GetUserControllerAccess(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error) {
	if j.GetUserControllerAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetUserControllerAccess_(ctx, user, controller)
}

func (j *PermissionManager) GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error) {
	if j.GetUserModelAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetUserModelAccess_(ctx, user, model)
}

func (j *PermissionManager) GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	if j.GrantAuditLogAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantAuditLogAccess_(ctx, user, targetUserTag)
}

func (j *PermissionManager) GrantCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	if j.GrantCloudAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantCloudAccess_(ctx, user, ct, ut, access)
}

func (j *PermissionManager) GrantModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	if j.GrantModelAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantModelAccess_(ctx, user, mt, ut, access)
}

func (j *PermissionManager) GrantOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
	if j.GrantOfferAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantOfferAccess_(ctx, user, offerURL, ut, access)
}

func (j *PermissionManager) GrantServiceAccountAccess(ctx context.Context, u *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
	if j.GrantServiceAccountAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantServiceAccountAccess_(ctx, u, svcAccTag, entities)
}

func (j *PermissionManager) RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	if j.RevokeAuditLogAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeAuditLogAccess_(ctx, user, targetUserTag)
}

func (j *PermissionManager) RevokeCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	if j.RevokeCloudAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeCloudAccess_(ctx, user, ct, ut, access)
}

func (j *PermissionManager) RevokeModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	if j.RevokeModelAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeModelAccess_(ctx, user, mt, ut, access)
}

func (j *PermissionManager) RevokeOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error) {
	if j.RevokeOfferAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeOfferAccess_(ctx, user, offerURL, ut, access)
}

func (j *PermissionManager) ToJAASTag(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error) {
	if j.ToJAASTag_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.ToJAASTag_(ctx, tag, resolveUUIDs)
}

func (j *PermissionManager) OpenFGACleanup(ctx context.Context) error {
	if j.OpenFGACleanup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.OpenFGACleanup_(ctx)
}
