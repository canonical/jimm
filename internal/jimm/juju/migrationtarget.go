// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/juju/core/migration"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// AbortMigration aborts a model migration with the given model UUID.
// It does this by calling the Abort methodon the target Juju controller.
// It also deletes the migration record from the database, but does not return an error
// if the deletion fails, as the migration has already been aborted on the target controller.
func (j *JujuManager) AbortMigration(ctx context.Context, user *openfga.User, modelUUID string) error {
	const op = errors.Op("jimm.Abort")

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get model migration %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &incomingModel.TargetController)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.Abort(modelUUID)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to abort migration: %w", err))
	}

	err = j.Database.DeleteIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		// Don't return an error if we fail to delete the migration record,
		// as the migration has already been aborted on the target controller.
		zapctx.Error(ctx, "failed to delete incoming model migration", zap.Error(err), zap.String("modelUUID", modelUUID))
	}
	return nil
}

// CheckMachines checks the machines in the model with the given UUID
// and compares them with the ones reported by the provider.
// It calls the CheckMachines method on the target Juju controller.
func (j *JujuManager) CheckMachines(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error) {
	const op = errors.Op("jimm.CheckMachines")

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return nil, errors.E(op, fmt.Errorf("failed to get model migration %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &incomingModel.TargetController)
	if err != nil {
		return nil, errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	machineErrors, err := api.CheckMachines(modelUUID)
	if err != nil {
		return nil, errors.E(op, fmt.Errorf("failed to check machines: %w", err))
	}
	return machineErrors, nil
}

// ControllerDetailsForIncomingModel retrieves the target controller details for a model that is being migrated.
// It returns the controller information, username, and password for the target controller.
func (j *JujuManager) ControllerDetailsForIncomingModel(ctx context.Context, modelUUID string) (ControllerConnectionDetails, error) {
	const op = errors.Op("jimm.ControllerDetailsForIncomingModel")

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}

	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return ControllerConnectionDetails{}, errors.E(op, errors.CodeNotFound, fmt.Sprintf("migrating model %q not found", modelUUID))
		}
		return ControllerConnectionDetails{}, errors.E(op, fmt.Errorf("failed to get controller for model %q: %w", modelUUID, err))
	}

	username, password, err := j.CredentialStore.GetControllerCredentials(ctx, incomingModel.TargetController.Name)
	if err != nil {
		return ControllerConnectionDetails{}, err
	}

	if username == "" || password == "" {
		return ControllerConnectionDetails{}, errors.E(op, errors.CodeNotFound, fmt.Errorf("missing credentials for controller %q", incomingModel.TargetController.Name))
	}

	return toControllerConnectionDetails(incomingModel.TargetController, username, password), nil
}

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

	newOwnerTag := names.NewUserTag(newOwner)
	model.Owner = newOwnerTag
	model.ModelDescription.SetOwner(newOwnerTag)
	model.ModelDescription.SetUsers(nil) // Clear users with access since JIMM gates access.
	return nil
}

// LatestLogTime asks the target controller for the time of the latest
// log record it has seen.
func (j *JujuManager) LatestLogTime(ctx context.Context, modelUUID string) (time.Time, error) {
	const op = errors.Op("jimm.LatestLogTime")

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return time.Time{}, errors.E(op, fmt.Errorf("failed to get model %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &model.Controller)
	if err != nil {
		return time.Time{}, errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	t, err := api.LatestLogTime(modelUUID)
	if err != nil {
		return time.Time{}, errors.E(op, fmt.Errorf("failed to get latest log time for model %q: %w", modelUUID, err))
	}
	return t, nil
}

// Activate gets the model migration, proxies the Activate call to the target controller,
// and then deletes the model migration from the database.
func (j *JujuManager) Activate(ctx context.Context, modelTag names.ModelTag, migrationInfo coremigration.SourceControllerInfo, relatedModels []string) error {
	const op = errors.Op("jimm.Activate")

	modelMigration := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get model migration for model %q: %w", modelTag.Id(), err))
	}
	api, err := j.dialController(ctx, &modelMigration.TargetController)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.Activate(modelTag.Id(), migrationInfo, relatedModels)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to activate model %q: %w", modelTag.Id(), err))
	}

	// This is done in a transaction to ensure that the model migration is only deleted
	// if user mappings have been created.
	err = j.Database.Transaction(func(db *db.Database) error {
		for localUser, externalUser := range modelMigration.UserMapping {
			userMapping := &dbmodel.UserMapping{
				ModelUUID:        modelMigration.ModelUUID,
				LocalUser:        localUser,
				ExternalUserName: externalUser,
			}
			err = db.AddUserMapping(ctx, userMapping)
			if err != nil {
				return errors.E(op, fmt.Errorf("failed to add user mapping for model %q: %w", modelTag.Id(), err))
			}
		}
		// TODO(SimoneDutto): set the model we've created in Import to active.
		err = db.DeleteIncomingModelMigration(ctx, &modelMigration)
		if err != nil {
			return errors.E(op, fmt.Errorf("failed to delete model migration for model %q: %w", modelTag.Id(), err))
		}
		return nil
	})
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to activate model %q: %w", modelTag.Id(), err))
	}
	return nil
}
