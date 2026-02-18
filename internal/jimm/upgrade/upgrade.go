// Copyright 2025 Canonical.

// upgrade package provides functionality to manage the upgrade process
// for controllers in JIMM.
package upgrade

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

var (
	AlreadyUpgradedError = jujuerrors.New("model has already been upgraded")
)

// JujuManager defines the juju manager methods required by the upgrade manager.
type JujuManager interface {
	ListMigrationTargets(ctx context.Context, user *openfga.User, modelTag names.ModelTag) ([]dbmodel.Controller, error)
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	ModelInfo(ctx context.Context, user *openfga.User, mt names.ModelTag) (jujuclient.ModelInfo, error)
}

// Store defines the store methods required by the upgrade manager.
type Store interface {
	GetModel(ctx context.Context, model *dbmodel.Model) (err error)
}

// UpgradeEnqueuer defines the method to enqueue an upgrade job.
type UpgradeEnqueuer interface {
	EnqueueUpgradeTo(ctx context.Context, args rivertypes.UpgradeToArgs) (*rivertype.JobInsertResult, error)
}

// UpgradeManager provides a means to manage controller upgrades within JIMM.
type UpgradeManager struct {
	jujuManager JujuManager
	store       Store
	dialer      juju.Dialer
	enqueuer    UpgradeEnqueuer
}

// NewUpgradeManager creates a new UpgradeManager instance.
func NewUpgradeManager(
	jujumanager JujuManager,
	store Store,
	dialer juju.Dialer,
	enqueuer UpgradeEnqueuer,
) (*UpgradeManager, error) {
	if jujumanager == nil {
		return nil, errors.E("juju manager cannot be nil")
	}
	if store == nil {
		return nil, errors.E("store cannot be nil")
	}
	if dialer == nil {
		return nil, errors.E("dialer cannot be nil")
	}
	if enqueuer == nil {
		return nil, errors.E("enqueuer cannot be nil")
	}
	return &UpgradeManager{
		jujuManager: jujumanager,
		store:       store,
		dialer:      dialer,
		enqueuer:    enqueuer,
	}, nil
}

// UpgradeModel upgrades the model to the provided agent version.
func (u *UpgradeManager) UpgradeModel(ctx context.Context, modelUUID string, targetVersion version.Number) error {
	ctx = zapctx.WithFields(ctx, zap.String("model_uuid", modelUUID), zap.String("target_version", targetVersion.String()))

	// Forbid a zero target version as this complicates checking for whether
	// the upgrade was successful.
	if targetVersion == version.Zero {
		return errors.E(errors.CodeBadRequest, "target version cannot be zero")
	}

	model := &dbmodel.Model{UUID: sql.NullString{Valid: true, String: modelUUID}}
	if err := u.store.GetModel(ctx, model); err != nil {
		return errors.E(errors.CodeNotFound, err, "model not found")
	}

	api, err := u.dialer.Dial(ctx, &model.Controller, names.ModelTag{}, nil, nil)
	if err != nil {
		return errors.E(fmt.Errorf("failed to dial target controller: %w", err))
	}

	if err := retry.Call(
		retry.CallArgs{
			Attempts: 6,
			Delay:    10 * time.Second,
			Func: func() error {
				mi, err := api.ModelInfo(ctx, model.ResourceTag())
				if err != nil {
					return fmt.Errorf("failed to get model info before upgrade: %w", err)
				}
				if mi.AgentVersion == nil {
					return fmt.Errorf("model agent version is nil before upgrade")
				}
				if mi.AgentVersion.Compare(targetVersion) >= 0 {
					// Already at target version or newer.
					zapctx.Info(ctx, "model upgrade done - agent version at or newer than target", zap.String("current_version", mi.AgentVersion.String()))
					return nil
				}

				// UpgradeModel is safe to call multiple times.
				_, err = api.UpgradeModel(modelUUID, targetVersion, "", false, false)
				if jujuparams.IsCodeUpgradeInProgress(err) {
					err = errors.E("upgrade in progress")
				}
				if jujuerrors.Is(err, jujuerrors.AlreadyExists) {
					// Model is already upgraded
					return nil
				}
				if err != nil {
					return errors.E(fmt.Errorf("failed to upgrade model: %w", err))
				}

				return errors.E(fmt.Errorf("model upgrade started/in-progress"))
			},
			NotifyFunc: func(lastError error, attempt int) {
				zapctx.Debug(ctx, "model upgrade attempt", zap.Error(lastError), zap.Int("attempt", attempt))
			},
			Clock: clock.WallClock,
		},
	); err != nil {
		return errors.E(fmt.Errorf("failed to complete upgrade: %w", err))
	}
	return nil
}

// UpgradeTo upgrades a model.
// That is, it bootstraps a new controller with the target version
// and inserts a River job to migrate and upgrade the model to the new controller.
// It returns the River job ID.
//
// This currently only works with non-kubernetes clouds.
func (u *UpgradeManager) UpgradeTo(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string) (int64, error) {
	if !names.IsValidModel(modelUUID) {
		return 0, errors.E(errors.CodeBadRequest, "invalid model UUID")
	}
	mt := names.NewModelTag(modelUUID)

	// Check that the target controller is a valid target for this model.
	validControllers, err := u.jujuManager.ListMigrationTargets(ctx, user, mt)
	if err != nil {
		return 0, fmt.Errorf("failed to list migration targets: %w", err)
	}

	var targetController *dbmodel.Controller
	for _, c := range validControllers {
		if c.Name == targetControllerName {
			targetController = &c
			break
		}
	}
	if targetController == nil {
		return 0, errors.E(errors.CodeBadRequest, fmt.Sprintf("target controller %s is not a valid migration target for this model", targetControllerName))
	}

	targetVersion, err := version.Parse(targetController.AgentVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to parse target controller version: %w", err)
	}

	job, err := u.enqueuer.EnqueueUpgradeTo(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		TargetVersion:        targetVersion,
		Username:             user.Name,
		TargetControllerName: targetControllerName,
	})
	if err != nil {
		return 0, errors.E(fmt.Errorf("failed to enqueue model migration and upgrade job: %w", err))
	}

	if job.UniqueSkippedAsDuplicate {
		return 0, errors.E("an upgrade job for this model is already in progress", errors.CodeInProgress)
	}

	return job.Job.ID, nil
}

// MigrateModel migrates a model to a new controller without upgrading the model's agent.
//
// If the model is already on the target controller, no action is taken and nil is returned.
func (u *UpgradeManager) MigrateModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string) error {
	ctx = zapctx.WithFields(ctx, zap.String("model_uuid", modelUUID), zap.String("target_controller_name", targetControllerName))

	if !names.IsValidModel(modelUUID) {
		return errors.E(errors.CodeBadRequest, "invalid model UUID")
	}

	// Fetch the model info to refresh current controller.
	mt := names.NewModelTag(modelUUID)
	_, err := u.jujuManager.ModelInfo(ctx, user, mt)
	if err != nil {
		return err
	}

	m, err := u.jujuManager.GetModel(ctx, modelUUID)
	if err != nil {
		return errors.E(err)
	}
	if m.Controller.Name == targetControllerName {
		zapctx.Info(ctx, "model is already on target controller, skipping migration", zap.String("model-uuid", modelUUID), zap.String("controller-name", targetControllerName))
		return nil
	}

	zapctx.Debug(ctx, "Attempting to initiate internal migration")
	_, err = u.jujuManager.InitiateInternalMigration(ctx, user, modelUUID, targetControllerName)
	if err != nil {
		return errors.E(fmt.Errorf("failed to initiate internal migration: %w", err))
	}

	modelNotMigratedErr := errors.E("model has not yet migrated to target controller")

	if err := retry.Call(
		retry.CallArgs{
			IsFatalError: func(err error) bool {
				return !stderrors.Is(err, modelNotMigratedErr)
			},
			Attempts: 30,
			Delay:    10 * time.Second,
			Func: func() error {
				mi, err := u.jujuManager.ModelInfo(ctx, user, mt)
				if err != nil {
					return err
				}

				m, err := u.jujuManager.GetModel(ctx, modelUUID)
				if err != nil {
					return err
				}

				if m.Controller.Name == targetControllerName {
					return nil
				}

				if mi.MigrationStatus != nil && mi.MigrationStatus.End != nil {
					endUTC := mi.MigrationStatus.End.UTC().Format(time.RFC3339)
					return fmt.Errorf("model migration failed: migration ended at %s with status %s", endUTC, mi.MigrationStatus.Status)
				}

				return modelNotMigratedErr
			},
			NotifyFunc: func(lastError error, attempt int) {
				zapctx.Debug(ctx, "model migrate attempt", zap.Error(lastError), zap.Int("attempt", attempt))
			},
			Clock: clock.WallClock,
		},
	); err != nil {
		return errors.E(fmt.Errorf("failed to confirm internal migration completed: %w", err))
	}

	return nil
}
