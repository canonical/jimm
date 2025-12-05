// Copyright 2025 Canonical.

// upgrade package provides functionality to manage the upgrade process
// for controllers in JIMM.
package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
)

var (
	AlreadyUpgradedError = jujuerrors.New("model has already been upgraded")
)

// BootstrapManager defines the bootstrap manager methods required by the upgrade manager.
type BootstrapManager interface {
	WaitForJobCompletion(ctx context.Context, jobId uuid.UUID, config bootstrap.WaitConfig) error
	StartBootstrapJob(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error)
}

// JujuManager defines the juju manager methods required by the upgrade manager.
type JujuManager interface {
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	ModelInfo(ctx context.Context, user *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error)
}

// Store defines the store methods required by the upgrade manager.
type Store interface {
	GetController(ctx context.Context, controller *dbmodel.Controller) (err error)
}

// upgradeManager provides a means to manage controller upgrades within JIMM.
type upgradeManager struct {
	bootstrapManager BootstrapManager
	jujuManager      JujuManager
	store            Store
	dialer           juju.Dialer
}

// NewUpgradeManager creates a new UpgradeManager instance.
func NewUpgradeManager(
	bootstrapManager BootstrapManager,
	jujumanager JujuManager,
	store Store,
	dialer juju.Dialer,
) (*upgradeManager, error) {
	if bootstrapManager == nil {
		return nil, errors.E("bootstrap manager cannot be nil")
	}
	if jujumanager == nil {
		return nil, errors.E("juju manager cannot be nil")
	}
	if store == nil {
		return nil, errors.E("store cannot be nil")
	}
	if dialer == nil {
		return nil, errors.E("dialer cannot be nil")
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
func (u *upgradeManager) PrepareUpgradeTo(ctx context.Context, modelUUID string, targetVersion version.Number) (jujucloud.Cloud, string, jujucloud.Credential, error) {
	var bootstrapCloud jujucloud.Cloud
	var bootstrapCloudRegion string
	var bootstrapCredential jujucloud.Credential

	m, err := u.jujuManager.GetModel(ctx, modelUUID)
	if err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(err)
	}

	currentVersion, err := version.Parse(m.Controller.AgentVersion)
	if err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(err)
	}

	if currentVersion.Compare(targetVersion) == 1 {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(errors.CodeBadRequest, "target version must be greater than or equal to current version")
	}

	api, err := u.dialer.Dial(ctx, &m.Controller, names.ModelTag{}, nil, nil)
	if err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("failed to dial the controller: %w", err))
	}

	var ctrlModelSummary jujuparams.ModelSummary
	if err := api.ControllerModelSummary(ctx, &ctrlModelSummary); err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("failed to get controller model summary: %w", err))
	}

	// TODO(ale8k): Handle K8S clouds here in future. (HostCloudRegion field.)
	bootstrapCloudRegion = ctrlModelSummary.CloudRegion

	ctrlCloud, err := names.ParseCloudTag(ctrlModelSummary.CloudTag)
	if err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("failed to parse cloud tag from controller model summary: %w", err))
	}

	ctrlCloudCred, err := names.ParseCloudCredentialTag(ctrlModelSummary.CloudCredentialTag)
	if err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("failed to parse cloud credential tag from controller model summary: %w", err))
	}

	credentialContents, err := api.CredentialContents(ctrlCloud.Id(), ctrlCloudCred.Name(), true)
	if err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("failed to get credential contents from controller model summary: %w", err))
	}

	// The client actually returns an error if this is 0 and no error returned, but to be defensive we're checking
	// anyways.
	if len(credentialContents) == 0 {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E("no credential contents found for controller cloud credential")
	}

	if credentialContents[0].Error != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("credential content error: %w", credentialContents[0].Error))
	}

	bootstrapCredential = jujucloud.NewCredential(
		jujucloud.AuthType(credentialContents[0].Result.Content.AuthType),
		credentialContents[0].Result.Content.Attributes,
	)

	if err := api.Cloud(ctrlCloud, &bootstrapCloud); err != nil {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E(fmt.Errorf("failed to get cloud from controller model summary: %w", err))
	}

	if !bootstrapCloud.IsControllerCloud {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E("controller cloud is not marked as a controller cloud")
	}

	if !bootstrapCloud.IsControllerCloud {
		return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, errors.E("controller cloud is not marked as a controller cloud")
	}

	return bootstrapCloud, bootstrapCloudRegion, bootstrapCredential, nil
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
func (u *upgradeManager) MigrateAndUpgradeModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string, targetVersion version.Number) (version.Number, error) {
	var controllerChosenVersion version.Number

	// TODO: Once precheck PR merged, run precheck.

	// As the controller has just been bootstrapped, the controller machine can still be in a pending state.
	// Unfortunately Juju doesn't return a typed error for us to examine, and the best we've got is the error message string.
	// So we retry a few times here to allow the controller machine to come up.
	var iimResult jujuparams.InitiateMigrationResult
	if err := retry.Call(
		retry.CallArgs{
			Attempts: 10,
			Delay:    5 * time.Second,
			// We could consider all errors fatal, bar: target prechecks failed: machine 0 not running (pending)
			// IsFatalError:
			Func: func() error {
				zapctx.Debug(ctx, "Attempting to initiate internal migration")
				r, err := u.jujuManager.InitiateInternalMigration(ctx, user, modelUUID, targetControllerName)
				if err != nil {
					zapctx.Error(ctx, "Failed to initiate internal migration", zap.Error(err))
					return err
				}

				iimResult = r
				return nil
			},
			Clock: clock.WallClock,
		},
	); err != nil {
		return controllerChosenVersion, errors.E(fmt.Errorf("failed to initiate internal migration: %w", err))
	}

	mt, err := names.ParseModelTag(iimResult.ModelTag)
	if err != nil {
		return controllerChosenVersion, errors.E(fmt.Errorf("failed to parse model tag from initiate internal migration result: %w", err))
	}

	var mi *jujuparams.ModelInfo
	if err := retry.Call(
		retry.CallArgs{
			Attempts: 10,
			Delay:    5 * time.Second,
			Func: func() error {
				mi, err = u.jujuManager.ModelInfo(ctx, user, mt)
				if err != nil {
					return err
				}

				m, err := u.jujuManager.GetModel(ctx, mi.UUID)
				if err != nil {
					return err
				}

				// It hasn't migrated yet, so error out.
				if m.Controller.Name != targetControllerName {
					return errors.E("model has not yet migrated to target controller")
				}

				return nil
			},
			Clock: clock.WallClock,
		},
	); err != nil {
		return controllerChosenVersion, errors.E(fmt.Errorf("failed to confirm internal migration completed: %w", err))
	}

	dbCtrl := &dbmodel.Controller{Name: targetControllerName}
	if err := u.store.GetController(ctx, dbCtrl); err != nil {
		return controllerChosenVersion, errors.E(errors.CodeNotFound, err, "controller not found")
	}

	api, err := u.dialer.Dial(ctx, dbCtrl, names.ModelTag{}, nil, nil)
	if err != nil {
		return controllerChosenVersion, errors.E(fmt.Errorf("failed to dial target controller: %w", err))
	}

	var upgradeErr error
	controllerChosenVersion, upgradeErr = api.UpgradeModel(mi.UUID, targetVersion, "", false, true)
	//nolint:staticcheck // Has a description to highlight this possibility.
	if jujuparams.IsCodeUpgradeInProgress(upgradeErr) {
		// Apparently upgrades can have issues, that can be manually resolved, then you can run
		// the upgrade-model command with the --reset-previous-upgrade option.
		// We can't do that here, so we just return an error. We could possible indicate to the user here
		// that manual intervention is required.
	}
	if jujuerrors.Is(upgradeErr, jujuerrors.AlreadyExists) {
		upgradeErr = AlreadyUpgradedError
	}

	if upgradeErr != nil {
		return controllerChosenVersion, errors.E(fmt.Errorf("failed to upgrade model after migration: %w", upgradeErr))
	}

	zapctx.Info(ctx, "model migrate and upgrade complete",
		zap.String("model-uuid", modelUUID),
		zap.String("target-controller", targetControllerName),
		zap.String("target-version", targetVersion.String()),
		zap.String("controller-chosen-version", controllerChosenVersion.String()),
	)
	return controllerChosenVersion, nil
}

// UpgradeTo upgrades a model.
// That is, it bootstraps a new controller with the target version
// and migrates the model to that controller. This is "Phase 1" of the
// automated upgrade process.
//
// This currently only works with personal, non-kubernetes clouds.
// Further work in Phase 2 is expected to be done here: https://warthogs.atlassian.net/browse/JUJU-8918
func (u *upgradeManager) UpgradeTo(ctx context.Context, user *openfga.User, modelUUID string, targetVersion version.Number) (version.Number, error) {
	var chosenVersion version.Number
	var newControllerName string = fmt.Sprintf("controller-%d", time.Now().Unix())

	bsCloud, bsCloudRegion, bsCredential, err := u.PrepareUpgradeTo(ctx, modelUUID, targetVersion)
	if err != nil {
		return chosenVersion, errors.E(fmt.Errorf("failed to prepare for upgrade: %w", err))
	}

	cloneParams := CloneControllerParams{
		CLIVersion:         targetVersion.String(),
		CloudNameAndRegion: fmt.Sprintf("%s/%s", bsCloud.Name, bsCloudRegion),
		ControllerName:     newControllerName,
		CloudCred:          bsCredential,
	}

	// TODO: Check if it is public cloud here and set PersonalCloud accordingly.
	// For now, we're always setting it.
	cloneParams.PersonalCloud = bsCloud

	// TODO: If K8S, override CloudNameAndRegion with HostCloudRegion from controller model summary.

	// TODO: Map user config from source controller here.

	if err := u.CloneController(ctx, user, cloneParams); err != nil {
		return chosenVersion, errors.E(fmt.Errorf("failed to clone controller: %w", err))
	}

	chosenVersion, err = u.MigrateAndUpgradeModel(
		ctx,
		user,
		modelUUID,
		cloneParams.ControllerName,
		targetVersion,
	)
	if err != nil {
		return chosenVersion, errors.E(fmt.Errorf("failed to migrate and upgrade model: %w", err))
	}

	return chosenVersion, nil
}
