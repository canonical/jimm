// Copyright 2025 Canonical.

// upgrade package provides functionality to manage the upgrade process
// for controllers in JIMM.
package upgrade

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
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
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("failed to dial the controller: %w", err))
	}

	var ctrlModelSummary jujuparams.ModelSummary
	if err := api.ControllerModelSummary(ctx, &ctrlModelSummary); err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("failed to get controller model summary: %w", err))
	}

	ctrlCloud, err := names.ParseCloudTag(ctrlModelSummary.CloudTag)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("failed to parse cloud tag from controller model summary: %w", err))
	}
	ctrlCloudCred, err := names.ParseCloudCredentialTag(ctrlModelSummary.CloudCredentialTag)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("failed to parse cloud credential tag from controller model summary: %w", err))
	}

	credentialContents, err := api.CredentialContents(ctrlCloud.Id(), ctrlCloudCred.Id(), true)
	if err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("failed to get credential contents from controller model summary: %w", err))
	}

	// The client actually returns an error if this is 0 and no error returned, but to be defensive we're checking
	// anyways.
	if len(credentialContents) == 0 {
		return bootstrapCloud, bootstrapCredential, errors.E("no credential contents found for controller cloud credential")
	}

	if credentialContents[0].Error != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("credential content error: %w", credentialContents[0].Error))
	}

	bootstrapCredential = jujucloud.NewCredential(
		jujucloud.AuthType(credentialContents[0].Result.Content.AuthType),
		credentialContents[0].Result.Content.Attributes,
	)

	if err := api.Cloud(ctrlCloud, &bootstrapCloud); err != nil {
		return bootstrapCloud, bootstrapCredential, errors.E(fmt.Errorf("failed to get cloud from controller model summary: %w", err))
	}

	if !bootstrapCloud.IsControllerCloud {
		return bootstrapCloud, bootstrapCredential, errors.E("controller cloud is not marked as a controller cloud")
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
