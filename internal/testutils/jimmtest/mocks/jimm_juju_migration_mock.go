// Copyright 2025 Canonical.
package mocks

import (
	"context"
	"time"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/semversion"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type MigrationMocks struct {
	Prechecks_            func(ctx context.Context, user *openfga.User, model juju.MigratingModelInfo) error
	PrechecksV2_          func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error
	AdoptResources_       func(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion semversion.Number) error
	Activate_             func(ctx context.Context, user *openfga.User, modelUUID names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error
	AbortMigration_       func(ctx context.Context, user *openfga.User, modelUUID string) error
	CheckMachines_        func(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error)
	Import_               func(ctx context.Context, user *openfga.User, serialized jujuparams.SerializedModel) error
	ImportV2_             func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error
	LatestLogTime_        func(ctx context.Context, user *openfga.User, modelUUID string) (time.Time, error)
	ListMigrationTargets_ func(ctx context.Context, user *openfga.User, modelTag names.ModelTag) ([]dbmodel.Controller, error)
}

func (j *MigrationMocks) AbortMigration(ctx context.Context, user *openfga.User, modelUUID string) error {
	if j.AbortMigration_ == nil {
		return errors.New("not implemented")
	}
	return j.AbortMigration_(ctx, user, modelUUID)
}

func (j *MigrationMocks) Prechecks(ctx context.Context, user *openfga.User, model juju.MigratingModelInfo) error {
	if j.Prechecks_ == nil {
		return errors.New("not implemented")
	}
	return j.Prechecks_(ctx, user, model)
}

func (j *MigrationMocks) AdoptResources(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion semversion.Number) error {
	if j.AdoptResources_ == nil {
		return errors.New("not implemented")
	}
	return j.AdoptResources_(ctx, user, modelUUID, sourceControllerVersion)
}

func (j *MigrationMocks) Activate(ctx context.Context, user *openfga.User, modelTag names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error {
	if j.Activate_ == nil {
		return errors.New("not implemented")
	}
	return j.Activate_(ctx, user, modelTag, sourceControllerInfo, relatedModels)
}

func (j *MigrationMocks) CheckMachines(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error) {
	if j.CheckMachines_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.CheckMachines_(ctx, user, modelUUID)
}

func (j *MigrationMocks) LatestLogTime(ctx context.Context, user *openfga.User, modelUUID string) (time.Time, error) {
	if j.LatestLogTime_ == nil {
		return time.Time{}, errors.New("not implemented")
	}
	return j.LatestLogTime_(ctx, user, modelUUID)
}

func (j *MigrationMocks) Import(ctx context.Context, user *openfga.User, serialized jujuparams.SerializedModel) error {
	if j.Import_ == nil {
		return errors.New("not implemented")
	}
	return j.Import_(ctx, user, serialized)
}

func (j *MigrationMocks) ListMigrationTargets(ctx context.Context, user *openfga.User, modelTag names.ModelTag) ([]dbmodel.Controller, error) {
	if j.ListMigrationTargets_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.ListMigrationTargets_(ctx, user, modelTag)
}

func (j *MigrationMocks) PrechecksV2(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
	if j.PrechecksV2_ == nil {
		return errors.New("not implemented")
	}
	return j.PrechecksV2_(ctx, user, envelope)
}

func (j *MigrationMocks) ImportV2(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
	if j.ImportV2_ == nil {
		return errors.New("not implemented")
	}
	return j.ImportV2_(ctx, user, envelope)
}
