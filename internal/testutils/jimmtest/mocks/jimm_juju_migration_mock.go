// Copyright 2025 Canonical.
package mocks

import (
	"context"

	"github.com/juju/juju/core/migration"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type MigrationMocks struct {
	Prechecks_      func(ctx context.Context, user *openfga.User, model migration.ModelInfo) error
	AdoptResources_ func(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion version.Number) error
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
