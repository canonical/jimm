// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/juju/core/migration"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// Prechecks checks that the model can be migrated to the target controller.
// It does this by calling the method of the same name on the target Juju controller.
// As part of all model migrations passing through JIMM, it modifies the model description
// to replace any local user references with their external mapping.
func (j *JujuManager) Prechecks(ctx context.Context, user *openfga.User, model migration.ModelInfo) error {
	const op = errors.Op("jimm.Prechecks")

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: model.UUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get model migration %q: %w", model.UUID, err))
	}

	err = j.modifyMigrationInfo(&model, incomingModel.UserMapping)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to modify migration info: %w", err))
	}

	api, err := j.dialController(ctx, &incomingModel.TargetController)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.Prechecks(model)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to run pre-checks for migration: %w", err))
	}
	return nil
}

// AdoptResources adopts resources from a model with the given UUID
// and controller version. This is used to adopt resources from a
// model that is being migrated. It calls the method of the same name
// on the target Juju controller.
func (j *JujuManager) AdoptResources(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion version.Number) error {
	const op = errors.Op("jimm.AdoptResources")

	modelMigration := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get model migration for model %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &modelMigration.TargetController)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.AdoptResources(modelUUID, sourceControllerVersion)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to adopt resources: %w", err))
	}
	return nil
}

// modifyMigrationInfo modifies the description of the model migration
// to replace any local user references with their external mapping.
func (j *JujuManager) modifyMigrationInfo(model *migration.ModelInfo, userMapping dbmodel.StringMap) error {
	if !model.Owner.IsLocal() {
		// If the owner is not a local user, we do not modify it.
		// This is useful when migrating a model from one JIMM
		// controller to another, where the owner is already an external user.
		return nil
	}
	newOwner, ok := userMapping[model.Owner.Id()]
	if !ok {
		// If the owner is not found in the user mappings, we return an error.
		// This is to ensure that the migration does not proceed with an invalid owner.
		return errors.E(fmt.Errorf("no external user mapping found for local user %q", model.Owner.Id()))
	}
	model.Owner = names.NewUserTag(newOwner)
	// TODO: Replace fields on the model description including the model owner and users.

	return nil
}
