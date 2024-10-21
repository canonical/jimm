// Copyright 2024 Canonical.
package mocks

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// ModelManager defines the mock struct used to implement the ModelManger interface.
type ModelManager struct {
	AddModel_               func(ctx context.Context, u *openfga.User, args *jimm.ModelCreateArgs) (*jujuparams.ModelInfo, error)
	ChangeModelCredential_  func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error
	DestroyModel_           func(ctx context.Context, u *openfga.User, mt names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error
	DumpModel_              func(ctx context.Context, u *openfga.User, mt names.ModelTag, simplified bool) (string, error)
	DumpModelDB_            func(ctx context.Context, u *openfga.User, mt names.ModelTag) (map[string]interface{}, error)
	ForEachModel_           func(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	ForEachUserModel_       func(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	FullModelStatus_        func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error)
	GetModel_               func(ctx context.Context, uuid string) (dbmodel.Model, error)
	ImportModel_            func(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error
	IdentityModelDefaults_  func(ctx context.Context, user *dbmodel.Identity) (map[string]interface{}, error)
	ModelDefaultsForCloud_  func(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error)
	ModelInfo_              func(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error)
	ModelStatus_            func(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error)
	QueryModelsJq_          func(ctx context.Context, models []string, jqQuery string) (params.CrossModelQueryResponse, error)
	SetModelDefaults_       func(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, configs map[string]interface{}) error
	UnsetModelDefaults_     func(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, keys []string) error
	UpdateMigratedModel_    func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error
	ValidateModelUpgrade_   func(ctx context.Context, u *openfga.User, mt names.ModelTag, force bool) error
	WatchAllModelSummaries_ func(ctx context.Context, controller *dbmodel.Controller) (_ func() error, err error)
}

func (j *ModelManager) AddModel(ctx context.Context, u *openfga.User, args *jimm.ModelCreateArgs) (_ *jujuparams.ModelInfo, err error) {
	if j.AddModel_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AddModel_(ctx, u, args)
}

func (j *ModelManager) ChangeModelCredential(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error {
	if j.ChangeModelCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ChangeModelCredential_(ctx, user, modelTag, cloudCredentialTag)
}

func (j *ModelManager) DestroyModel(ctx context.Context, u *openfga.User, mt names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error {
	if j.DestroyModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.DestroyModel_(ctx, u, mt, destroyStorage, force, maxWait, timeout)
}

func (j *ModelManager) DumpModel(ctx context.Context, u *openfga.User, mt names.ModelTag, simplified bool) (string, error) {
	if j.DumpModel_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.DumpModel_(ctx, u, mt, simplified)
}
func (j *ModelManager) DumpModelDB(ctx context.Context, u *openfga.User, mt names.ModelTag) (map[string]interface{}, error) {
	if j.DumpModelDB_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.DumpModelDB_(ctx, u, mt)
}

func (j *ModelManager) ForEachModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	if j.ForEachModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachModel_(ctx, u, f)
}

func (j *ModelManager) ForEachUserModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	if j.ForEachUserModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachUserModel_(ctx, u, f)
}

func (j *ModelManager) FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error) {
	if j.FullModelStatus_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FullModelStatus_(ctx, user, modelTag, patterns)
}

func (j *ModelManager) GetModel(ctx context.Context, uuid string) (dbmodel.Model, error) {
	if j.GetModel_ == nil {
		return dbmodel.Model{}, errors.E(errors.CodeNotImplemented)
	}
	return j.GetModel_(ctx, uuid)
}

func (j *ModelManager) ImportModel(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error {
	if j.ImportModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ImportModel_(ctx, user, controllerName, modelTag, newOwner)
}

func (j *ModelManager) ModelDefaultsForCloud(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error) {
	if j.ModelDefaultsForCloud_ == nil {
		return jujuparams.ModelDefaultsResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.ModelDefaultsForCloud_(ctx, user, cloudTag)
}

func (j *ModelManager) ModelInfo(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error) {
	if j.ModelInfo_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ModelInfo_(ctx, u, mt)
}
func (j *ModelManager) ModelStatus(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error) {
	if j.ModelStatus_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ModelStatus_(ctx, u, mt)
}

func (j *ModelManager) QueryModelsJq(ctx context.Context, models []string, jqQuery string) (params.CrossModelQueryResponse, error) {
	if j.QueryModelsJq_ == nil {
		return params.CrossModelQueryResponse{}, errors.E(errors.CodeNotImplemented)
	}
	return j.QueryModelsJq_(ctx, models, jqQuery)
}

func (j *ModelManager) SetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, configs map[string]interface{}) error {
	if j.SetModelDefaults_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetModelDefaults_(ctx, user, cloudTag, region, configs)
}

func (j *ModelManager) UnsetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, keys []string) error {
	if j.UnsetModelDefaults_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UnsetModelDefaults_(ctx, user, cloudTag, region, keys)
}

func (j *ModelManager) UpdateMigratedModel(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error {
	if j.UpdateMigratedModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateMigratedModel_(ctx, user, modelTag, targetControllerName)
}
func (j *ModelManager) IdentityModelDefaults(ctx context.Context, user *dbmodel.Identity) (map[string]interface{}, error) {
	if j.IdentityModelDefaults_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.IdentityModelDefaults_(ctx, user)
}
func (j *ModelManager) ValidateModelUpgrade(ctx context.Context, u *openfga.User, mt names.ModelTag, force bool) error {
	if j.ValidateModelUpgrade_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ValidateModelUpgrade_(ctx, u, mt, force)
}
func (j *ModelManager) WatchAllModelSummaries(ctx context.Context, controller *dbmodel.Controller) (_ func() error, err error) {
	if j.WatchAllModelSummaries_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.WatchAllModelSummaries_(ctx, controller)
}
