// Copyright 2025 Canonical.
package mocks

import (
	"context"

	"github.com/juju/juju/core/migration"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type MigrationMocks struct {
	Prechecks_ func(ctx context.Context, user *openfga.User, model migration.ModelInfo) error
}

func (j *MigrationMocks) Prechecks(ctx context.Context, user *openfga.User, model migration.ModelInfo) error {
	if j.Prechecks_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.Prechecks_(ctx, user, model)
}
