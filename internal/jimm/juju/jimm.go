// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelupgrader"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/rpc/params"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"golang.org/x/sync/errgroup"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/openfga"
)

var (
	initiateInternalMigration = func(ctx context.Context, j *JujuManager, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
		internalMigration := true
		return j.initiateMigration(ctx, user, spec, internalMigration)
	}
)

// forEachController runs a given function on multiple controllers
// simultaneously. A connection is established to every controller in the
// given list concurrently and then the given function is called with the
// controller and API connection to use to perform the controller
// operation. ForEachConnection waits until all operations have finished
// before returning, any error returned will be the first error
// encountered when connecting to the controller or returned from the given
// function.
func (j *JujuManager) forEachController(ctx context.Context, controllers []dbmodel.Controller, f func(*dbmodel.Controller, API) error) error {
	eg := new(errgroup.Group)
	for i := range controllers {
		i := i
		eg.Go(func() error {
			api, err := j.dial(ctx, &controllers[i], names.ModelTag{}, nil)
			if err != nil {
				return err
			}
			defer api.Close()
			return f(&controllers[i], api)
		})
	}
	return eg.Wait()
}

// ControllerInfo returns info about a controller connected to JIMM.
func (j *JujuManager) ControllerInfo(ctx context.Context, name string) (*dbmodel.Controller, error) {

	ctl := dbmodel.Controller{
		Name: name,
	}
	if err := j.Database.GetController(ctx, &ctl); err != nil {
		return nil, errors.E(err)
	}
	return &ctl, nil
}

// ListControllers returns a list of controllers the user has access to.
func (j *JujuManager) ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	var controllers []dbmodel.Controller
	err := j.Database.ForEachController(ctx, func(c *dbmodel.Controller) error {
		controllers = append(controllers, *c)
		return nil
	})
	if err != nil {
		return nil, errors.E(err)
	}

	return controllers, nil
}

// SetControllerDeprecated records if the controller is to be deprecated.
// No new models or clouds can be added to a deprecated controller.
func (j *JujuManager) SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error {

	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err := j.Database.Transaction(func(db *db.Database) error {
		c := dbmodel.Controller{
			Name: controllerName,
		}
		if err := db.GetController(ctx, &c); err != nil {
			return err
		}
		c.Deprecated = deprecated
		return db.UpdateController(ctx, &c)
	})
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// RemoveController removes a controller.
func (j *JujuManager) RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error {

	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err := j.Database.Transaction(func(db *db.Database) error {
		c := dbmodel.Controller{
			Name: controllerName,
		}
		if err := db.GetController(ctx, &c); err != nil {
			return err
		}

		// if c.UnavailableSince is valid, then we can delete is
		// if c.UnavailableSince is no valid, then we can't delete is
		// if force is true, we can always delete is
		if !force && !c.UnavailableSince.Valid {
			return errors.E(errors.CodeStillAlive, "controller is still alive")
		}

		models, err := db.GetModelsByController(ctx, c)
		if err != nil {
			return err
		}
		// Delete its models first.
		for _, model := range models {
			err := db.DeleteModel(ctx, &model)
			if err != nil {
				return err
			}
		}

		// Then delete the controller
		return db.DeleteController(ctx, &c)
	})
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// FullModelStatus returns the full status of the juju model.
func (j *JujuManager) FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return nil, errors.E(err)
	}

	api, err := j.dial(ctx, &model.Controller, modelTag, nil)
	if err != nil {
		return nil, errors.E(err)
	}

	status, err := api.Status(ctx, patterns)
	if err != nil {
		return nil, errors.E(err)
	}

	return status, nil
}

type migrationControllerID = uint

func fillMigrationTarget(db *db.Database, credStore credentials.CredentialStore, controllerName string) (jujuparams.MigrationTargetInfo, migrationControllerID, error) {
	dbController := dbmodel.Controller{
		Name: controllerName,
	}
	ctx := context.Background()
	err := db.GetController(ctx, &dbController)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return jujuparams.MigrationTargetInfo{}, 0, err
		}
		return jujuparams.MigrationTargetInfo{}, 0, errors.E(err, fmt.Errorf("failed to get controller with name %q", controllerName))
	}
	adminUser, adminPass, err := credStore.GetControllerCredentials(ctx, controllerName)
	if err != nil {
		return jujuparams.MigrationTargetInfo{}, 0, err
	}
	if adminUser == "" || adminPass == "" {
		return jujuparams.MigrationTargetInfo{}, 0, errors.E("missing target controller credentials")
	}
	// Should we verify controller can access the cloud where the model is currently hosted?
	apiControllerInfo := dbController.ToAPIControllerInfo()
	targetInfo := jujuparams.MigrationTargetInfo{
		ControllerAlias: dbController.Name, // This value will be returned to us on successful migration.
		ControllerTag:   dbController.ResourceTag().String(),
		Addrs:           apiControllerInfo.APIAddresses,
		CACert:          dbController.CACertificate,
		// The target user must be the admin user as external users don't have username/password credentials.
		AuthTag:  names.NewUserTag(adminUser).String(),
		Password: adminPass,
	}
	return targetInfo, dbController.ID, nil
}

// InitiateInternalMigration initiates a model migration between two controllers within JIMM.
func (j *JujuManager) InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error) {

	migrationTarget, _, err := fillMigrationTarget(j.Database, j.CredentialStore, targetController)
	if err != nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(err)
	}

	model := dbmodel.Model{}
	// Check if the user is providing a model UUID or name
	_, err = uuid.Parse(modelNameOrUUID)
	if err != nil {
		s := strings.Split(modelNameOrUUID, "/")
		if len(s) != 2 {
			return jujuparams.InitiateMigrationResult{}, errors.E("invalid model target")
		}

		owner, name := s[0], s[1]
		if !names.IsValidUser(owner) {
			return jujuparams.InitiateMigrationResult{}, errors.E("invalid user name")
		}
		if !names.IsValidModelName(name) {
			return jujuparams.InitiateMigrationResult{}, errors.E("invalid model name")
		}

		model.Name = name
		model.OwnerIdentityName = owner
	} else {
		model.UUID = sql.NullString{
			String: modelNameOrUUID,
			Valid:  true,
		}
	}

	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(err)
	}
	spec := jujuparams.MigrationSpec{ModelTag: model.ResourceTag().String(), TargetInfo: migrationTarget}
	result, err := initiateInternalMigration(ctx, j, user, spec)
	if err != nil {
		return result, errors.E(err)
	}
	return result, nil
}

// PrepareModelMigration takes the model ID from the migrating controller and stores a record
// in the IncomingModelMigation table to prepare it for migration against the target controller's name.
func (j *JujuManager) PrepareModelMigration(
	ctx context.Context,
	user *openfga.User,
	modelUUID string,
	targetControllerName string,
	userMapping map[string]string,
) (string, error) {

	err := j.Database.Transaction(func(d *db.Database) error {
		ctl := dbmodel.Controller{Name: targetControllerName}
		if err := d.GetController(ctx, &ctl); err != nil {
			return err
		}

		// Verify the model doesn't exist - if it does that means a migration is
		// in progress or completed or it could also mean the model failed to be removed
		// during migration ABORT but that problem should be dealt with separately.
		model := &dbmodel.Model{
			UUID: sql.NullString{String: modelUUID, Valid: true},
		}
		err := d.GetModel(ctx, model)
		if err == nil {
			return errors.E("model migration for the specified model is already in progress/completed")
		} else if errors.ErrorCode(err) != errors.CodeNotFound {
			return err
		}

		if err := d.AddOrUpdateIncomingModelMigration(ctx, &dbmodel.IncomingModelMigration{
			ModelUUID:          sql.NullString{String: modelUUID, Valid: true},
			TargetControllerID: ctl.ID,
			UserMapping:        dbmodel.StringMap(userMapping),
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return "", errors.E(fmt.Errorf("failed to add incoming model migration details: %w", err))
	}

	migrationToken, err := j.migrationTokenGenerator.NewMigrationToken(ctx, user.Name)
	if err != nil {
		return "", errors.E(fmt.Errorf("failed to generate migration token: %w", err))
	}

	return migrationToken, nil
}

// ListMigrationTargets returns the list of juju controllers that the given internal
// model could be migrated to. This includes controllers that support the model's
// cloud region and version, but excludes the controller the model is already on.
func (j *JujuManager) ListMigrationTargets(ctx context.Context, user *openfga.User, modelTag names.ModelTag) ([]dbmodel.Controller, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	var model dbmodel.Model
	model.SetTag(modelTag)
	if err := j.Database.GetModel(ctx, &model); err != nil {
		return nil, errors.E(err)
	}

	currentVersion, err := version.Parse(model.Controller.AgentVersion)
	if err != nil {
		return nil, errors.E(err)
	}

	cloudRegion, err := j.Database.FindRegionByCloudName(ctx, model.CloudRegion.CloudName, model.CloudRegion.Name)
	if err != nil {
		return nil, errors.E(err)
	}

	var controllers []dbmodel.Controller
	for _, ctl := range cloudRegion.Controllers {
		candidateVersion, err := version.Parse(ctl.Controller.AgentVersion)
		if err != nil {
			return nil, errors.E(err)
		}

		if model.Controller.ID != ctl.Controller.ID &&
			currentVersion.Compare(candidateVersion) <= 0 {
			controllers = append(controllers, ctl.Controller)
		}
	}

	return controllers, nil
}

// PrepareUpgradeTo prepares the necessary cloud and credential information
// to perform a controller upgrade to the specified target version and validates
// the target version is greater than the current controller version.
//
// It returns the cloud and credential to be used for bootstrapping
// the new controller.
func (j *JujuManager) PrepareUpgradeTo(ctx context.Context, modelUUID string, targetVersion version.Number) (jujucloud.Cloud, jujucloud.Credential, error) {
	var bootstrapCloud jujucloud.Cloud
	var bootstrapCredential jujucloud.Credential

	m, err := j.GetModel(ctx, modelUUID)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(err)
	}

	currentVersion, err := version.Parse(m.Controller.AgentVersion)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(err)
	}

	if currentVersion.Compare(targetVersion) >= 0 {
		return bootstrapCloud, bootstrapCredential, errors.E(errors.CodeBadRequest, "target version must be greater than current version")
	}

	api, err := j.dial(ctx, &m.Controller, names.ModelTag{}, nil)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("failed to dial the controller", err)
	}

	// TODO: When adding controller, import controller models, then we can find the controller model
	// based on it's UUID via ModelInfo and not iterate through model summaries checking IsController.
	ctrlModelSummary, err := getControllerModelSummary(ctx, api)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("failed to get controller model summary", err)
	}

	cloudClient := cloud.NewClient(api)

	ctrlCloud, err := names.ParseCloudTag(ctrlModelSummary.CloudTag)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("failed to parse cloud tag from controller model summary", err)
	}
	ctrlCloudCred, err := names.ParseCloudCredentialTag(ctrlModelSummary.CloudCredentialTag)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("failed to parse cloud credential tag from controller model summary", err)
	}

	credentialContents, err := cloudClient.CredentialContents(ctrlCloud.Id(), ctrlCloudCred.Id(), true)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("failed to get credential contents from controller model summary", err)
	}

	if len(credentialContents) == 0 {
		return bootstrapCloud, bootstrapCredential, errors.E("no credential contents found for controller cloud credential")
	}

	if credentialContents[0].Error != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("credential content error", credentialContents[0].Error)
	}

	bootstrapCredential = jujucloud.NewCredential(
		jujucloud.AuthType(credentialContents[0].Result.Content.AuthType),
		credentialContents[0].Result.Content.Attributes,
	)

	// TODO: Instead of credential contents, perhaps this is OK?
	// cloudCredRes, err := cloudClient.Credentials(ctrlCloudCred)

	bootstrapCloud, err = cloudClient.Cloud(ctrlCloud)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E("failed to get cloud from controller model summary", err)
	}

	return bootstrapCloud, bootstrapCredential, nil
}

// MigrateAndUpgradeModel migrates a model to a new controller and upgrades the model's agent to the controller's agent version.
func (j *JujuManager) MigrateAndUpgradeModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string, targetVersion version.Number) error {
	// TODO: Once precheck PR merged, run precheck.

	iimResult, err := j.InitiateInternalMigration(ctx, user, modelUUID, targetControllerName)
	if err != nil {
		return errors.E("failed to initiate internal migration for upgrade", err)
	}

	mt, err := names.ParseModelTag(iimResult.ModelTag)
	if err != nil {
		return errors.E("failed to parse model tag from initiate internal migration result", err)
	}

	var mi *jujuparams.ModelInfo
	if err := retry.Call(
		retry.CallArgs{
			Attempts: 10,
			Delay:    5 * time.Second,
			Func: func() error {
				// TODO: We don't care about the user here. We just wanna know if internal migration completed.
				// Our ModelInfo handles the redirect, it'll error until the migration is complete (traversing the redirect err to
				// the new controller).
				mi, err = j.ModelInfo(ctx, user, mt)
				return err
			},
		},
	); err != nil {
		return errors.E("failed to confirm internal migration completed", err)
	}

	dbCtrl, err := j.getControllerByName(ctx, targetControllerName)
	if err != nil {
		return errors.E("failed to get target controller by name", err)
	}

	api, err := j.dialController(ctx, dbCtrl)
	if err != nil {
		return errors.E("failed to dial target controller", err)
	}

	// TODO: Backup the model and have recovery for failure scenarios here.

	modelUpgrader := modelupgrader.NewClient(api)
	// TODO: Do we wanna support AgentStream / Ignore certain versions?
	var upgradeErr error
	var controllerChosenVersion version.Number
	// TODO: What exactly is IgnoreAgentversions?
	// https://github.com/juju/juju/blob/09b9c7b8ff49f936997a84728a58b6a6f2eb66de/cmd/juju/commands/upgrademodel.go#L84
	controllerChosenVersion, upgradeErr = modelUpgrader.UpgradeModel(mi.UUID, targetVersion, "", false, true)
	if params.IsCodeUpgradeInProgress(upgradeErr) {
		// TODO: Return already in progress?
		// Apparently upgrades can have issues, that can be manually resolved, then you can run
		// the upgrade-model command with the --reset-previous-upgrade option.
	}
	if jujuerrors.Is(upgradeErr, jujuerrors.AlreadyExists) {
		upgradeErr = jujuerrors.New("model has already been upgraded")
	}

	if upgradeErr != nil {
		return errors.E("failed to upgrade model after migration", upgradeErr)
	}

	zapctx.Info(ctx, "model migrated and upgrade complete",
		zap.String("model-uuid", modelUUID),
		zap.String("target-controller", targetControllerName),
		zap.String("target-version", targetVersion.String()),
		zap.String("controller-chosen-version", controllerChosenVersion.String()),
	)
	return nil
}
