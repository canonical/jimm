// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	goerr "errors"
	"fmt"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/juju/core/migration"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

const TIMEOUT_PENDING_MIGRATION = 24 * time.Hour

// AbortMigration aborts a model migration with the given model UUID.
// It does this by calling the Abort method on the target Juju controller.
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
	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	// Don't return an error if we fail to delete the model from JIMM's state,
	// as the migration has already been aborted on the target controller.
	// The model will be cleanup eventually by JIMM's cleanup routine.
	err = j.Database.DeleteModel(ctx, &model)
	if err != nil {
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
// It does this by checking cloud, cloudregion and cloud credentials exists in JIMM, then
// calling the method of the same name on the target Juju controller.
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

	_, err = j.Database.FindRegionByCloudName(ctx, model.ModelDescription.CloudCredential().Cloud(), model.ModelDescription.CloudRegion())
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to find region for cloud %q: %w", model.ModelDescription.CloudCredential().Cloud(), err))
	}

	cloudCredential := &dbmodel.CloudCredential{
		CloudName:         model.ModelDescription.CloudCredential().Cloud(),
		OwnerIdentityName: model.ModelDescription.Owner().Id(),
		Name:              model.ModelDescription.CloudCredential().Name(),
	}

	err = j.Database.GetCloudCredential(ctx, cloudCredential)
	if err != nil {
		return errors.E(op, err)
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
//
// Adopt resources is called after the model has been activated so the
// incoming model migration does not exist and the model is used instead.
func (j *JujuManager) AdoptResources(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion version.Number) error {
	const op = errors.Op("jimm.AdoptResources")

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get model migration for model %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &model.Controller)
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
	err := j.modifyModelDescription(model.ModelDescription, userMapping)
	if err != nil {
		return errors.E(fmt.Errorf("failed to modify model description: %w", err))
	}
	return nil
}

// modifyModelDescription modifies the model description to replace local user references
// with their external mapping for both the model owner and the cloud credential owner.
func (j *JujuManager) modifyModelDescription(modelDescription description.Model, userMapping dbmodel.StringMap) error {
	// change the owner of the model description if it is a local user
	if modelDescription.Owner().IsLocal() {
		// If the owner is a local user, we replace it with the external mapping.
		newOwner, ok := userMapping[modelDescription.Owner().Id()]
		if !ok {
			return errors.E(fmt.Errorf("no external user mapping found for local user %q", modelDescription.Owner().Id()))
		}
		modelDescription.SetOwner(names.NewUserTag(newOwner))
	}

	modelDescription.SetUsers(nil)

	// change cloud credendial owner if it is a local user
	credentials := modelDescription.CloudCredential()
	if credentials == nil {
		return fmt.Errorf("model description must contain a cloud credential")
	}
	if !names.IsValidCloud(credentials.Cloud()) {
		return errors.E(fmt.Errorf("invalid cloud name %q", credentials.Cloud()))
	}
	cloudTag := names.NewCloudTag(credentials.Cloud())

	if !names.IsValidUser(credentials.Owner()) {
		return errors.E(fmt.Errorf("invalid cloud credential owner %q", credentials.Owner()))
	}
	ownerTag := names.NewUserTag(credentials.Owner())
	if ownerTag.IsLocal() {
		newOwner, ok := userMapping[ownerTag.Id()]
		if !ok {
			return errors.E(fmt.Errorf("no external user mapping found for cloud credential local user %q", modelDescription.Owner().Id()))
		}
		ownerTag = names.NewUserTag(newOwner)
	}

	modelDescription.SetCloudCredential(description.CloudCredentialArgs{
		Owner:      ownerTag,
		Name:       credentials.Name(),
		AuthType:   credentials.AuthType(),
		Attributes: credentials.Attributes(),
		Cloud:      cloudTag,
	})
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
		model := dbmodel.Model{
			UUID: sql.NullString{
				String: modelTag.Id(),
				Valid:  true,
			},
		}
		err = db.GetModel(ctx, &model)
		if err != nil {
			return errors.E(op, fmt.Errorf("failed to get model %q: %w", modelTag.Id(), err))
		}
		model.MigrationMode = state.MigrationModeNone
		model.Life = state.Alive.String()

		err = db.UpdateModel(ctx, &model)
		if err != nil {
			return errors.E(op, fmt.Errorf("failed to update model %q: %w", modelTag.Id(), err))
		}

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

// Import imports a model from a serialized description.
//   - Checks the incoming model migration record in the database.
//   - Modifies the model description to replace local user references with their external mapping for owner and
//     cloud credential owner.
//   - Imports the model into JIMM's state.
//   - Calls the import method on the target Juju controller to import the model.
func (j *JujuManager) Import(ctx context.Context, user *openfga.User, serialized params.SerializedModel) error {
	const op = errors.Op("jimm.Import")

	modelDescription, err := description.Deserialize(serialized.Bytes)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to deserialize model description: %w", err))
	}
	incomingMigration := &dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelDescription.Tag().Id(),
			Valid:  true,
		},
	}
	err = j.Database.GetIncomingModelMigration(ctx, incomingMigration)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to add incoming model migration: %w", err))
	}
	err = j.modifyModelDescription(modelDescription, incomingMigration.UserMapping)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to modify model description: %w", err))
	}
	err = j.importModelFromDescription(ctx, incomingMigration.TargetController.ID, modelDescription)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to import model from description: %w", err))
	}

	api, err := j.dialController(ctx, &incomingMigration.TargetController)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	serializedDescrition, err := description.Serialize(modelDescription)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to serialize model description: %w", err))
	}
	err = api.Import(serializedDescrition)
	if err != nil {
		// TODO: handle migration failure in a cleanup routine.
		return errors.E(op, fmt.Errorf("failed to import model: %w", err))
	}
	return nil
}

// importModelFromDescription imports a model into JIMM's state from a model description.
// It creates a new model record in the database with the given target controller ID and model description
// and sets the migration mode to importing.
// It also ensures that the cloud credential and region are present in the database.
func (j *JujuManager) importModelFromDescription(ctx context.Context, targetControllerID uint, description description.Model) error {
	op := errors.Op("jimm.importModelFromDescription")
	modelNameStr, ok := description.Config()[config.NameKey].(string)
	if !ok {
		return errors.E(op, fmt.Errorf("model config must contain a string value for key %q", config.NameKey))
	}
	// TODO: create the offers in JIMM's state. Card: https://warthogs.atlassian.net/browse/JUJU-8192

	modelUUIDStr, ok := description.Config()[config.UUIDKey].(string)
	if !ok {
		return errors.E(op, fmt.Errorf("model config must contain a string value for key %q", config.UUIDKey))
	}

	if description.CloudCredential() == nil {
		return errors.E(op, fmt.Errorf("model description must contain a cloud credential"))
	}
	cloudCredential := &dbmodel.CloudCredential{
		CloudName:         description.CloudCredential().Cloud(),
		OwnerIdentityName: description.Owner().Id(),
		Name:              description.CloudCredential().Name(),
	}

	err := j.Database.GetCloudCredential(ctx, cloudCredential)
	if err != nil {
		return errors.E(op, err)
	}
	region, err := j.Database.FindRegionByCloudName(ctx, description.CloudCredential().Cloud(), description.CloudRegion())
	if err != nil {
		return errors.E(op, err)
	}
	err = j.Database.AddModel(ctx, &dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUIDStr,
			Valid:  true,
		},
		Name:              modelNameStr,
		OwnerIdentityName: description.Owner().Id(),
		ControllerID:      targetControllerID,
		CloudCredentialID: cloudCredential.ID,
		CloudRegionID:     region.ID,
		MigrationMode:     state.MigrationModeImporting,
	})
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to add model %q: %w", modelUUIDStr, err))
	}
	return nil
}

// CleanupPartialModelMigrations cleans up any partial model migrations that have exceeded the timeout.
// It deletes the incoming model migration record, deletes the user mappings for the model,
// and deletes the model record from JIMM's state.
func (j *JujuManager) CleanupPartialModelMigrations(ctx context.Context) error {
	const op = errors.Op("jimm.CleanupPartialModelMigrations")

	// Get all incoming model migrations that have exceeded the timeout.
	migrations, err := j.Database.GetIncomingModelMigrationsCreatedBefore(ctx, time.Now().Add(-TIMEOUT_PENDING_MIGRATION))
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get incoming model migrations: %w", err))
	}
	var errs []error
	for _, migration := range migrations {
		err := j.cleanupPartialModelMigration(ctx, migration)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return goerr.Join(errs...)
}

// cleanupPartialModelMigration cleans up a partial model migration by deleting the incoming model migration record,
// deleting the user mappings for the model, and deleting the model record from JIMM's state.
func (j *JujuManager) cleanupPartialModelMigration(ctx context.Context, migration dbmodel.IncomingModelMigration) error {
	const op = errors.Op("jimm.cleanupPartialModelMigration")

	return j.Database.Transaction(func(db *db.Database) error {
		// Delete the incoming model migration record.
		err := j.Database.DeleteIncomingModelMigration(ctx, &migration)
		if err != nil {
			return errors.E(op, err)
		}

		// Delete user mappings for the model.
		err = j.Database.DeleteUserMappingsByModelUUID(ctx, migration.ModelUUID.String)
		if err != nil {
			return errors.E(op, err)
		}

		// Delete the model record from JIMM's state.
		model := dbmodel.Model{
			UUID: sql.NullString{
				String: migration.ModelUUID.String,
				Valid:  true,
			},
		}
		err = j.Database.DeleteModel(ctx, &model)
		if err != nil {
			return errors.E(op, err)
		}
		return nil
	})
}
