// Copyright 2025 Canonical.

package juju

import (
	"context"
	"fmt"

	"github.com/juju/juju/rpc/params"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// PollModels loops over models, contacting the respective controller
// and checking, based on the model's migration mode, if the model exists.
// If the model exists in JIMM's database, but not on the controller,
// it is deleted from JIMM's database.
func (j *JujuManager) PollModels(ctx context.Context) (err error) {
	const op = errors.Op("jimm.PollModels")
	zapctx.Info(ctx, string(op))
	durationObserver := servermon.DurationObserver(servermon.JimmMethodsDurationHistogram, string(op))
	defer durationObserver()

	// Step 1: Group models by controller
	controllerModels := make(map[string][]*dbmodel.Model)
	err = j.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		key := m.Controller.UUID
		controllerModels[key] = append(controllerModels[key], m)
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}

	// Step 2: Loop over controllers and process their models
	// This way we only dial each controller once.
	for controllerUUID, models := range controllerModels {
		if len(models) == 0 {
			continue
		}
		api, err := j.dialController(ctx, &models[0].Controller)
		if err != nil {
			zapctx.Error(ctx, "cannot dial controller", zap.String("controller", controllerUUID), zap.Error(err))
			continue
		}
		// Depending the model's migration mode, we either:
		// - Check if the model exists (MigrationModeNone)
		// - Check if the model has completed internal migration (MigrationModeMigrateInternal)
		// - Do nothing if the model is in any other migration mode (MigrationModeImporting, MigrationModeExporting)
		for _, m := range models {
			ctx := zapctx.WithFields(ctx,
				zap.String("model-owner", m.OwnerIdentityName),
				zap.String("model-name", m.Name),
				zap.String("migration-mode", string(m.MigrationMode)),
			)
			var err error

			switch m.MigrationMode {
			case dbmodel.MigrationModeNone:
				err = j.maybeCleanupModel(ctx, api, m)
			case dbmodel.MigrationModeMigrateInternal:
				err = j.checkModelMigratedInternal(ctx, api, m)
			}
			if err != nil {
				zapctx.Error(ctx, "error processing model", zap.Error(err))
				continue
			}
		}
	}
	return nil
}

// checkModelMigratedInternal checks if the model has been migrated from
// one controller managed by JIMM to another controller managed by JIMM.
func (j *JujuManager) checkModelMigratedInternal(ctx context.Context, api API, m *dbmodel.Model) error {
	const op = errors.Op("jimm.checkModelMigratedInternal")

	// Check if the model has completed a migration.
	// If modelInfo returns without an error, it definitely hasn't moved yet.
	modelInfo := &jujuparams.ModelInfo{UUID: m.UUID.String}
	err := api.ModelInfo(ctx, modelInfo)
	if err == nil {
		// If the migration end time is set, it means the model has
		// failed to migrate otherwise we'd expect a redirect error.
		if modelInfo.Migration.End != nil {
			m.MigrationFailed()
			if err := j.Database.UpdateModel(ctx, m); err != nil {
				return errors.E(fmt.Errorf("failed to update model after failed migration: %w", err))
			}
		}
		return nil
	}

	// Expect a redirect error if the model successfully migrated.
	// This is the error that Juju controllers return when a model has been migrated.

	isRedirectErr := errors.ErrorCode(err) == params.CodeRedirect
	if !isRedirectErr {
		return errors.E(op, fmt.Errorf("failed to get model info: %w", err))
	}

	// Parse the redirect error to get the new controller details.
	errInfo := errors.ErrorInfo(err)
	if errInfo == nil {
		return errors.E(op, fmt.Errorf("missing error info in redirect error: %w", err))
	}

	var redirectInfo params.RedirectErrorInfo
	err = params.Error{Info: errInfo}.UnmarshalInfo(&redirectInfo)
	if err != nil {
		return errors.E(op, fmt.Errorf("cannot unmarshal redirect error info: %w", err))
	}

	// We expect this controller will be known to JIMM.
	controller := dbmodel.Controller{Name: redirectInfo.ControllerAlias}
	err = j.Database.GetController(ctx, &controller)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get controller %q: %w", redirectInfo.ControllerAlias, err))
	}

	m.InternalMigrationSuccess(controller.ID)

	err = j.Database.UpdateModel(ctx, m)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to update model after migration: %w", err))
	}
	zapctx.Info(ctx, "model successfully migrated to controller", zap.String("model", m.UUID.String), zap.String("controller_name", controller.Name))
	return nil
}

// maybeCleanupModel checks if the model exists on the controller.
// This performs eventual cleanup of models that have been deleted through
// the API (since the deletion of a model is not immediate) and handles
// cases where the model was deleted directly on the underlying controller.
func (j *JujuManager) maybeCleanupModel(ctx context.Context, api API, m *dbmodel.Model) error {
	const op = errors.Op("jimm.maybeCleanupModel")

	err := api.ModelInfo(ctx, &jujuparams.ModelInfo{UUID: m.UUID.String})
	if err == nil {
		// If the call succeeds, the model exists and we can return.
		return nil
	}
	// Some versions of juju return unauthorized for models that cannot be found.
	modelDeleted := (errors.ErrorCode(err) == errors.CodeNotFound || errors.ErrorCode(err) == errors.CodeUnauthorized)
	if modelDeleted {
		if err := j.deleteModel(ctx, m.ResourceTag()); err != nil {
			return errors.E(op, fmt.Errorf("failed to delete model: %w", err))
		}
	} else {
		return errors.E(op, fmt.Errorf("failed to get model info: %w", err))
	}
	return nil
}
