// Copyright 2025 Canonical.

// upgrade package provides functionality to manage the upgrade process
// for controllers in JIMM.
package upgrade

import (
	"context"
	"fmt"
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
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	jimmjuju "github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// BootstrapManager defines the bootstrap manager methods required by the upgrade manager.
type BootstrapManager interface {
	WaitForJobCompletion(ctx context.Context, jobId uuid.UUID, config bootstrap.WaitConfig) error
	StartBootstrapJob(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error)
}

type JujuManager interface {
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	ModelInfo(ctx context.Context, user *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error)
}

type Store interface {
	GetController(ctx context.Context, controller *dbmodel.Controller) (err error)
}

// upgradeManager provides a means to manage controller upgrades within JIMM.
type upgradeManager struct {
	bootstrapManager BootstrapManager
	jujuManager      JujuManager
	store            Store
	dialer           jimmjuju.Dialer
}

// NewUpgradeManager creates a new UpgradeManager instance.
func NewUpgradeManager(
	bootstrapManager BootstrapManager,
	jujumanager JujuManager,
	store Store,
	dialer jimmjuju.Dialer,
) (*upgradeManager, error) {
	if bootstrapManager == nil {
		return nil, errors.E("bootstrap manager cannot be nil")
	}
	return &upgradeManager{
		bootstrapManager: bootstrapManager,
		jujuManager:      jujumanager,
		store:            store,
		dialer:           dialer,
	}, nil
}

// PrepareUpgradeTo prepares the necessary cloud and credential information
// to perform a controller upgrade to the specified target version and validates
// the target version is greater than the current controller version.
//
// It returns the cloud and credential to be used for bootstrapping
// the new controller.
func (j *upgradeManager) PrepareUpgradeTo(ctx context.Context, modelUUID string, targetVersion version.Number) (jujucloud.Cloud, jujucloud.Credential, error) {
	var bootstrapCloud jujucloud.Cloud
	var bootstrapCredential jujucloud.Credential

	m, err := j.jujuManager.GetModel(ctx, modelUUID)
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

	api, err := j.dialer.Dial(ctx, &m.Controller, names.ModelTag{}, nil, nil)
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

// CloneController upgrades a controller by fetching its configuration and initiating
// a bootstrap job with that configuration, then waits for the bootstrap to complete.
func (u *upgradeManager) CloneController(ctx context.Context, user *openfga.User, params CloneControllerParams) error {
	if user == nil {
		return errors.E("user cannot be nil")
	}

	zapctx.Info(ctx, "starting controller upgrade", zap.String("controller-name", params.ControllerName))

	// Start the bootstrap job
	jobId, err := u.bootstrapManager.StartBootstrapJob(ctx, user, bootstrap.BootstrapParams{
		CLIVersion:         params.CLIVersion,
		CloudNameAndRegion: params.CloudNameAndRegion,
		ControllerName:     params.ControllerName,
		CloudCred:          params.CloudCred,
		PersonalCloud:      params.PersonalCloud,
		UserConfig:         params.UserConfig,
	})
	if err != nil {
		return errors.E(fmt.Errorf("failed to start bootstrap job: %w", err))
	}
	parsedJobId, err := uuid.Parse(jobId)
	if err != nil {
		return errors.E(fmt.Errorf("failed to parse bootstrap job ID: %w", err))
	}
	// Wait for the bootstrap job to complete
	if err := u.bootstrapManager.WaitForJobCompletion(ctx, parsedJobId, bootstrap.WaitConfig{}); err != nil {
		return errors.E(fmt.Errorf("bootstrap job failed: %w", err))
	}

	return nil
}

// MigrateAndUpgradeModel migrates a model to a new controller and upgrades the model's agent to the controller's agent version.
func (j *upgradeManager) MigrateAndUpgradeModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string, targetVersion version.Number) error {
	// TODO: Once precheck PR merged, run precheck.

	iimResult, err := j.jujuManager.InitiateInternalMigration(ctx, user, modelUUID, targetControllerName)
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
				mi, err = j.jujuManager.ModelInfo(ctx, user, mt)
				return err
			},
		},
	); err != nil {
		return errors.E("failed to confirm internal migration completed", err)
	}

	dbCtrl := &dbmodel.Controller{Name: targetControllerName}
	if err := j.store.GetController(ctx, dbCtrl); err != nil {
		return errors.E(errors.CodeNotFound, err, "controller not found")
	}

	api, err := j.dialer.Dial(ctx, dbCtrl, names.ModelTag{}, nil, nil)
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

// getControllerModelSummary returns the controllers model summary.
func getControllerModelSummary(ctx context.Context, api jimmjuju.API) (jujuparams.ModelSummary, error) {
	var ms jujuparams.ModelSummary
	if err := api.ControllerModelSummary(ctx, &ms); err != nil {
		zapctx.Error(ctx, "failed to get model summary", zaputil.Error(err))
		return ms, err
	}
	return ms, nil
}
