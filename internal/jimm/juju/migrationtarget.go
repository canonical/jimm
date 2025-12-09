// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	goerr "errors"
	"fmt"
	"strings"
	"time"

	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs/config"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/description"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

const TIMEOUT_PENDING_MIGRATION = 24 * time.Hour

// AbortMigration aborts a model migration with the given model UUID.
// It does this by calling the Abort method on the target Juju controller.
// It also deletes the migration record from the database, but does not return an error
// if the deletion fails, as the migration has already been aborted on the target controller.
func (j *JujuManager) AbortMigration(ctx context.Context, user *openfga.User, modelUUID string) error {

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get model migration %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &incomingModel.TargetController)
	if err != nil {
		return errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.Abort(modelUUID)
	if err != nil {
		return errors.E(fmt.Errorf("failed to abort migration: %w", err))
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
	// Don't return an error if we fail to delete the model/permissions from
	// JIMM's state, the migration has already been aborted on the target controller.
	// The model will be cleanup eventually by JIMM's cleanup routine.
	err = j.Database.DeleteModel(ctx, &model)
	if err != nil {
		zapctx.Error(ctx, "failed to delete incoming model migration", zap.Error(err), zap.String("modelUUID", modelUUID))
	}
	err = j.OpenFGAClient.RemoveModel(ctx, model.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to remove model from OpenFGA", zap.Error(err), zap.String("modelUUID", modelUUID))
	}
	return nil
}

// CheckMachines checks the machines in the model with the given UUID
// and compares them with the ones reported by the provider.
// It calls the CheckMachines method on the target Juju controller.
func (j *JujuManager) CheckMachines(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error) {

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return nil, errors.E(fmt.Errorf("failed to get model migration %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &incomingModel.TargetController)
	if err != nil {
		return nil, errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	machineErrors, err := api.CheckMachines(modelUUID)
	if err != nil {
		return nil, errors.E(fmt.Errorf("failed to check machines: %w", err))
	}
	return machineErrors, nil
}

// ControllerDetailsForIncomingModel retrieves the target controller details for a model that is being migrated.
// It returns the controller information, username, and password for the target controller.
func (j *JujuManager) ControllerDetailsForIncomingModel(ctx context.Context, modelUUID string) (ControllerConnectionDetails, error) {

	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}

	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return ControllerConnectionDetails{}, errors.E(errors.CodeNotFound, fmt.Sprintf("migrating model %q not found", modelUUID))
		}
		return ControllerConnectionDetails{}, errors.E(fmt.Errorf("failed to get controller for model %q: %w", modelUUID, err))
	}

	username, password, err := j.CredentialStore.GetControllerCredentials(ctx, incomingModel.TargetController.Name)
	if err != nil {
		return ControllerConnectionDetails{}, err
	}

	if username == "" || password == "" {
		return ControllerConnectionDetails{}, errors.E(errors.CodeNotFound, fmt.Errorf("missing credentials for controller %q", incomingModel.TargetController.Name))
	}

	return toControllerConnectionDetails(incomingModel.TargetController, username, password), nil
}

// Prechecks checks that the model can be migrated to the target controller.
// It does this by checking cloud, cloudregion and cloud credentials exists in JIMM, then
// calling the method of the same name on the target Juju controller.
// As part of all model migrations passing through JIMM, it modifies the model description
// to replace any local user references with their external mapping.
func (j *JujuManager) Prechecks(ctx context.Context, user *openfga.User, model MigratingModelInfo) error {
	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: model.UUID,
			Valid:  true,
		},
	}

	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get model migration %q: %w", model.UUID, err))
	}

	targetControllerVersion, err := version.Parse(incomingModel.TargetController.AgentVersion)
	if err != nil {
		return errors.E(fmt.Errorf("failed to parse target controller agent version %q: %w", incomingModel.TargetController.AgentVersion, err))
	}

	modelDescription, err := description.Deserialize(model.RawModelDescription, targetControllerVersion)
	if err != nil {
		return errors.E(fmt.Errorf("failed to deserialize model description: %w", err))
	}

	err = j.validateUserMapping(modelDescription, incomingModel.UserMapping)
	if err != nil {
		return errors.E(fmt.Errorf("failed to validate user mapping: %w", err))
	}

	model.Owner, err = j.modifyMigrationInfo(modelDescription, incomingModel.UserMapping)
	if err != nil {
		return errors.E(fmt.Errorf("failed to modify migration info: %w", err))
	}

	_, err = j.Database.FindRegionByCloudName(ctx, modelDescription.CloudCredential().Cloud(), modelDescription.CloudRegion())
	if err != nil {
		return errors.E(fmt.Errorf("failed to find region for cloud %q: %w", modelDescription.CloudCredential().Cloud(), err))
	}

	cloudCredential := &dbmodel.CloudCredential{
		CloudName:         modelDescription.CloudCredential().Cloud(),
		OwnerIdentityName: modelDescription.Owner().Id(),
		Name:              modelDescription.CloudCredential().Name(),
	}

	err = j.Database.GetCloudCredential(ctx, cloudCredential)
	if err != nil {
		return errors.E(err)
	}

	api, err := j.dialController(ctx, &incomingModel.TargetController)
	if err != nil {
		return errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	serializedModel, err := modelDescription.Serialize()
	if err != nil {
		return errors.E(fmt.Errorf("failed to serialize model description: %w", err))
	}
	err = api.Prechecks(jujuparams.MigrationModelInfo{
		UUID:                   model.UUID,
		OwnerTag:               model.Owner.String(),
		Name:                   model.Name,
		AgentVersion:           model.AgentVersion,
		ControllerAgentVersion: model.AgentVersion,
		ModelDescription:       serializedModel,
	})
	if err != nil {
		return errors.E(fmt.Errorf("failed to run pre-checks for migration: %w", err))
	}
	return nil
}

// validateUserMapping checks that the provided user mapping contains all the users
// that either have access to the model or have access to any application offers in the model.
func (j *JujuManager) validateUserMapping(modelDescription description.Model, userMapping dbmodel.StringMap) error {
	var missingUserMessages []string

	modelUsers := modelDescription.Users()
	for _, user := range modelUsers {
		if user.Name().Id() == ofganames.EveryoneUser {
			continue
		}
		if _, ok := userMapping[user.Name().Id()]; !ok {
			missingUserMessages = append(missingUserMessages, fmt.Sprintf("expected user %q who has %s access to the model", user.Name().Id(), user.Access()))
		}
	}

	apps := modelDescription.Applications()
	for _, app := range apps {
		for _, offer := range app.Offers() {
			for user, access := range offer.ACL() {
				if user == ofganames.EveryoneUser {
					continue
				}
				if _, ok := userMapping[user]; !ok {
					missingUserMessages = append(missingUserMessages, fmt.Sprintf("expected user %q who has %s access to offer %q", user, access, offer.OfferName()))
				}
			}
		}
	}
	if len(missingUserMessages) > 0 {
		return fmt.Errorf("user mapping is missing the following users:\n%s", strings.Join(missingUserMessages, "\n"))
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

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get model migration for model %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &model.Controller)
	if err != nil {
		return errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.AdoptResources(modelUUID, sourceControllerVersion)
	if err != nil {
		return errors.E(fmt.Errorf("failed to adopt resources: %w", err))
	}
	return nil
}

// modifyMigrationInfo modifies the description of the model migration
// to replace any local user references with their external mapping.
// It returns the new owner of the model after modification.
func (j *JujuManager) modifyMigrationInfo(model description.Model, userMapping dbmodel.StringMap) (names.UserTag, error) {
	if !model.Owner().IsLocal() {
		// If the owner is not a local user, we do not modify it.
		// This is useful when migrating a model from one JIMM
		// controller to another, where the owner is already an external user.
		return model.Owner(), nil
	}

	newOwner, ok := userMapping[model.Owner().Id()]
	if !ok {
		// If the owner is not found in the user mappings, we return an error.
		// This is to ensure that the migration does not proceed with an invalid owner.
		return names.UserTag{}, errors.E(fmt.Errorf("no external user mapping found for local user %q", model.Owner().Id()))
	}
	if !names.IsValidUser(newOwner) {
		return names.UserTag{}, errors.E(fmt.Errorf("invalid external user mapping %q for local user %q", newOwner, model.Owner().Id()))
	}

	newOwnerTag := names.NewUserTag(newOwner)
	err := modifyModelDescription(model, userMapping)
	if err != nil {
		return names.UserTag{}, errors.E(fmt.Errorf("failed to modify model description: %w", err))
	}
	return newOwnerTag, nil
}

// modifyModelDescription modifies the model description to replace local user references
// with their external mapping for both the model owner and the cloud credential owner.
func modifyModelDescription(modelDescription description.Model, userMapping dbmodel.StringMap) error {
	// change the owner of the model description if it is a local user
	if modelDescription.Owner().IsLocal() {
		// If the owner is a local user, we replace it with the external mapping.
		newOwner, ok := userMapping[modelDescription.Owner().Id()]
		if !ok {
			return errors.E(fmt.Errorf("no external user mapping found for local user %q", modelDescription.Owner().Id()))
		}
		modelDescription.SetOwner(names.NewUserTag(newOwner))
	}

	modelDescription.ClearUsers()

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

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUID,
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return time.Time{}, errors.E(fmt.Errorf("failed to get model %q: %w", modelUUID, err))
	}

	api, err := j.dialController(ctx, &model.Controller)
	if err != nil {
		return time.Time{}, errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	t, err := api.LatestLogTime(modelUUID)
	if err != nil {
		return time.Time{}, errors.E(fmt.Errorf("failed to get latest log time for model %q: %w", modelUUID, err))
	}
	return t, nil
}

// Activate gets the model migration, proxies the Activate call to the target controller,
// and then deletes the model migration from the database.
func (j *JujuManager) Activate(ctx context.Context, modelTag names.ModelTag, migrationInfo coremigration.SourceControllerInfo, relatedModels []string) error {

	modelMigration := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get model migration for model %q: %w", modelTag.Id(), err))
	}
	api, err := j.dialController(ctx, &modelMigration.TargetController)
	if err != nil {
		return errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	err = api.Activate(modelTag.Id(), migrationInfo, relatedModels)
	if err != nil {
		return errors.E(fmt.Errorf("failed to activate model %q: %w", modelTag.Id(), err))
	}

	// This is done in a transaction to ensure that the model migration is only deleted
	// if user mappings have been created.
	err = j.Database.Transaction(func(db *db.Database) error {
		for localUser, externalUser := range modelMigration.UserMapping {
			if externalUser == "" {
				// An empty external user indicates the user intentionally wants
				// to skip mapping the local user to an external user.
				continue
			}
			userMapping := &dbmodel.UserMapping{
				ModelUUID:        modelMigration.ModelUUID,
				LocalUser:        localUser,
				ExternalUserName: externalUser,
			}
			err = db.AddUserMapping(ctx, userMapping)
			if err != nil {
				return errors.E(fmt.Errorf("failed to add user mapping for model %q: %w", modelTag.Id(), err))
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
			return errors.E(fmt.Errorf("failed to get model %q: %w", modelTag.Id(), err))
		}
		model.MigrationMode = dbmodel.MigrationModeNone
		model.Life = state.Alive.String()

		err = db.UpdateModel(ctx, &model)
		if err != nil {
			return errors.E(fmt.Errorf("failed to update model %q: %w", modelTag.Id(), err))
		}

		err = db.DeleteIncomingModelMigration(ctx, &modelMigration)
		if err != nil {
			return errors.E(fmt.Errorf("failed to delete model migration for model %q: %w", modelTag.Id(), err))
		}
		return nil
	})
	if err != nil {
		return errors.E(fmt.Errorf("failed to activate model %q: %w", modelTag.Id(), err))
	}
	return nil
}

// Import imports a model from a serialized description.
//   - Checks the incoming model migration record in the database.
//   - Modifies the model description to replace local user references with their external mapping for owner and
//     cloud credential owner.
//   - Imports the model into JIMM's state.
//   - Adds permissions for the model and application offers.
//   - Calls the import method on the target Juju controller to import the model.
func (j *JujuManager) Import(ctx context.Context, user *openfga.User, serialized jujuparams.SerializedModel) error {

	// Determine the model UUID from the serialized description
	// and later use the model UUID to get the target controller
	// version so that we re-encode the description correctly.
	modelUUID, err := description.TryDetermineModelUUID(serialized.Bytes)
	if err != nil {
		return errors.E(fmt.Errorf("failed to determine model UUID: %w", err))
	}

	var (
		model             *dbmodel.Model
		offers            []*dbmodel.ApplicationOffer
		incomingMigration *dbmodel.IncomingModelMigration
		modelDescription  description.Model
	)

	// Start a transaction to acquire the incoming model migration record with a
	// lock to prevent it from being modified while we are importing the model.
	// Then import the model and app offers into JIMM's state - the existence
	// of the model implies that the migration record can no longer be modified.
	err = j.Database.Transaction(func(d *db.Database) error {
		incomingMigration = &dbmodel.IncomingModelMigration{
			ModelUUID: sql.NullString{String: modelUUID, Valid: true},
		}

		// Set noWait to false to allow the transaction to wait for the lock.
		noWait := false
		err = d.GetIncomingModelMigrationWithLock(ctx, incomingMigration, noWait)
		if err != nil {
			return errors.E(fmt.Errorf("failed to get incoming model migration: %w", err))
		}

		controllerVersion, err := version.Parse(incomingMigration.TargetController.AgentVersion)
		if err != nil {
			return errors.E(fmt.Errorf("failed to parse target controller agent version %q: %w", incomingMigration.TargetController.AgentVersion, err))
		}

		modelDescription, err = description.Deserialize(serialized.Bytes, controllerVersion)
		if err != nil {
			return errors.E(fmt.Errorf("failed to deserialize model description: %w", err))
		}

		err = modifyModelDescription(modelDescription, incomingMigration.UserMapping)
		if err != nil {
			return errors.E(fmt.Errorf("failed to modify model description: %w", err))
		}

		model, offers, err = importFromDescription(ctx, d, incomingMigration.TargetController.ID, modelDescription)
		if err != nil {
			return errors.E(fmt.Errorf("failed to import model from description: %w", err))
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Pass the controller tag as the controller details
	// are not populated on the model after creation.
	controllerTag := incomingMigration.TargetController.ResourceTag()
	err = j.addModelAndOfferPermissions(ctx, user, model, offers, controllerTag)
	if err != nil {
		return errors.E(fmt.Errorf("failed to add resource permissions: %w", err))
	}

	// Call the import method on the target controller to import the model.
	api, err := j.dialController(ctx, &incomingMigration.TargetController)
	if err != nil {
		return errors.E(fmt.Errorf("failed to dial controller: %w", err))
	}
	defer api.Close()

	serializedDescrition, err := modelDescription.Serialize()
	if err != nil {
		return errors.E(fmt.Errorf("failed to serialize model description: %w", err))
	}
	err = api.Import(serializedDescrition)
	if err != nil {
		// TODO: handle migration failure in a cleanup routine.
		return errors.E(fmt.Errorf("failed to import model: %w", err))
	}

	return nil
}

// importFromDescription imports resources into JIMM's state from a model description.
// It creates a new model record in the database with the given target controller ID
// and model description and sets the migration mode to importing.
// Application offers are created for any offers in the model description.
// It also ensures that the cloud credential and region are present in the database.
func importFromDescription(ctx context.Context, tx *db.Database, targetControllerID uint, description description.Model) (*dbmodel.Model, []*dbmodel.ApplicationOffer, error) {
	modelNameStr, ok := description.Config()[config.NameKey].(string)
	if !ok {
		return nil, nil, errors.E(fmt.Errorf("model config must contain a string value for key %q", config.NameKey))
	}

	modelUUIDStr, ok := description.Config()[config.UUIDKey].(string)
	if !ok {
		return nil, nil, errors.E(fmt.Errorf("model config must contain a string value for key %q", config.UUIDKey))
	}

	if description.CloudCredential() == nil {
		return nil, nil, errors.E(fmt.Errorf("model description must contain a cloud credential"))
	}
	cloudCredential := &dbmodel.CloudCredential{
		CloudName:         description.CloudCredential().Cloud(),
		OwnerIdentityName: description.Owner().Id(),
		Name:              description.CloudCredential().Name(),
	}

	err := tx.GetCloudCredential(ctx, cloudCredential)
	if err != nil {
		return nil, nil, errors.E(err)
	}
	region, err := tx.FindRegionByCloudName(ctx, description.CloudCredential().Cloud(), description.CloudRegion())
	if err != nil {
		return nil, nil, errors.E(err)
	}

	var importedModel *dbmodel.Model
	var importedOffers []*dbmodel.ApplicationOffer

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelUUIDStr,
			Valid:  true,
		},
		Name:              modelNameStr,
		OwnerIdentityName: description.Owner().Id(),
		ControllerID:      targetControllerID,
		CloudCredentialID: cloudCredential.ID,
		CloudRegionID:     region.ID,
		MigrationMode:     dbmodel.MigrationModeImporting,
	}
	err = tx.AddModel(ctx, &model)
	if err != nil {
		return nil, nil, errors.E(fmt.Errorf("failed to add model %q: %w", modelUUIDStr, err))
	}
	importedModel = &model

	for _, app := range description.Applications() {
		for _, offer := range app.Offers() {
			// construct the offer URL with the same logic as Juju (modelOwner, modelName, offerName, <blank-controller-name>)
			offerURL := jujucrossmodel.MakeURL(description.Owner().Id(), modelNameStr, offer.OfferName(), "")

			dbOffer := dbmodel.ApplicationOffer{
				UUID:    offer.OfferUUID(),
				Name:    offer.OfferName(),
				URL:     offerURL,
				ModelID: model.ID,
			}
			if err := tx.AddApplicationOffer(ctx, &dbOffer); err != nil {
				if errors.ErrorCode(err) == errors.CodeAlreadyExists {
					return nil, nil, fmt.Errorf("offer with URL %s already exists", dbOffer.URL)
				}
				return nil, nil, errors.E(fmt.Errorf("failed to add application offer %q: %w", dbOffer.Name, err))
			}

			importedOffers = append(importedOffers, &dbOffer)
		}
	}

	return importedModel, importedOffers, nil
}

// addModelAndOfferPermissions grants the user access to the model
// and adds the necesary relations between the model and app offers.
func (j *JujuManager) addModelAndOfferPermissions(ctx context.Context, user *openfga.User, model *dbmodel.Model, offers []*dbmodel.ApplicationOffer, ct names.ControllerTag) error {

	modelTag := model.ResourceTag()
	if err := j.addModelPermissions(ctx, user, modelTag, ct); err != nil {
		return errors.E(fmt.Errorf("failed to add model permissions: %w", err))
	}

	for _, offer := range offers {
		err := j.OpenFGAClient.AddModelApplicationOffer(ctx, modelTag, offer.ResourceTag())
		if err != nil {
			return errors.E(fmt.Errorf("failed to add application offer permissions: %w", err))
		}
	}
	return nil
}

// CleanupPartialModelMigrations cleans up any partial model migrations that have exceeded the timeout.
// It deletes the incoming model migration record, deletes the user mappings for the model,
// and deletes the model record from JIMM's state.
func (j *JujuManager) CleanupPartialModelMigrations(ctx context.Context) error {

	// Get all incoming model migrations that have exceeded the timeout.
	migrations, err := j.Database.GetIncomingModelMigrationsCreatedBefore(ctx, time.Now().Add(-TIMEOUT_PENDING_MIGRATION))
	if err != nil {
		return errors.E(fmt.Errorf("failed to get incoming model migrations: %w", err))
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

	return j.Database.Transaction(func(db *db.Database) error {
		// Delete the incoming model migration record.
		err := j.Database.DeleteIncomingModelMigration(ctx, &migration)
		if err != nil {
			return errors.E(err)
		}

		// Delete user mappings for the model.
		err = j.Database.DeleteUserMappingsByModelUUID(ctx, migration.ModelUUID.String)
		if err != nil {
			return errors.E(err)
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
			return errors.E(err)
		}
		return nil
	})
}
