// Copyright 2025 Canonical.
package mocks

import (
	"context"

	"github.com/juju/juju/core/migration"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type MigrationMocks struct {
	Prechecks_      func(ctx context.Context, user *openfga.User, model migration.ModelInfo) error
	AdoptResources_ func(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion version.Number) error
	Activate_       func(ctx context.Context, modelUUID names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error
	AbortMigration_ func(ctx context.Context, user *openfga.User, modelUUID string) error
	CheckMachines_  func(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error)
}

func (j *MigrationMocks) AbortMigration(ctx context.Context, user *openfga.User, modelUUID string) error {
	if j.AbortMigration_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AbortMigration_(ctx, user, modelUUID)
}

func (j *MigrationMocks) Prechecks(ctx context.Context, user *openfga.User, model migration.ModelInfo) error {
	if j.Prechecks_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.Prechecks_(ctx, user, model)
}

func (j *MigrationMocks) AdoptResources(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion version.Number) error {
	if j.AdoptResources_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AdoptResources_(ctx, user, modelUUID, sourceControllerVersion)
}

func (j *MigrationMocks) Activate(ctx context.Context, modelTag names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error {
	if j.Activate_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.Activate_(ctx, modelTag, sourceControllerInfo, relatedModels)
}

func (j *MigrationMocks) CheckMachines(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error) {
	if j.CheckMachines_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.CheckMachines_(ctx, user, modelUUID)
}
