// Copyright 2025 Canonical.

package juju

import (
	"context"
	"fmt"

	"github.com/juju/juju/rpc/params"
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

			_, err := j.modelInfo(ctx, m, api)
			if err != nil {
				zapctx.Error(ctx, "error getting model info", zap.Error(err))
			}
		}
	}
	return nil
}

// checkModelMigratedInternal checks if the model has been migrated from
// one controller managed by JIMM to another controller managed by JIMM.
func (j *JujuManager) checkModelMigratedInternal(ctx context.Context, errFromAPI error, m *dbmodel.Model) error {
	const op = errors.Op("jimm.checkModelMigratedInternal")

	// Expect a redirect error if the model successfully migrated.
	// This is the error that Juju controllers return when a model has been migrated.
	isRedirectErr := errors.ErrorCode(errFromAPI) == params.CodeRedirect
	if !isRedirectErr {
		return errors.E(op, errFromAPI)
	}

	// Parse the redirect error to get the new controller details.
	errInfo := errors.ErrorInfo(errFromAPI)
	if errInfo == nil {
		return errors.E(op, fmt.Errorf("missing error info in redirect error: %w", errFromAPI))
	}

	var redirectInfo params.RedirectErrorInfo
	err := params.Error{Info: errInfo}.UnmarshalInfo(&redirectInfo)
	if err != nil {
		return errors.E(op, fmt.Errorf("cannot unmarshal redirect error info: %w", err))
	}

	// We expect this controller will be known to JIMM.
	controller := dbmodel.Controller{Name: redirectInfo.ControllerAlias}
	err = j.Database.GetController(ctx, &controller)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to get controller %q: %w", redirectInfo.ControllerAlias, errFromAPI))
	}

	m.InternalMigrationSuccess(controller.ID)

	err = j.Database.UpdateModel(ctx, m)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to update model after migration: %w", errFromAPI))
	}
	zapctx.Info(ctx, "model successfully migrated to controller", zap.String("model", m.UUID.String), zap.String("controller_name", controller.Name))
	return nil
}

// maybeCleanupModel checks if the model exists on the controller.
// This performs eventual cleanup of models that have been deleted through
// the API (since the deletion of a model is not immediate) and handles
// cases where the model was deleted directly on the underlying controller.
func (j *JujuManager) maybeCleanupModel(ctx context.Context, errFromAPI error, m *dbmodel.Model) error {
	const op = errors.Op("jimm.maybeCleanupModel")
	// Some versions of juju return unauthorized for models that cannot be found.
	modelDeleted := (errors.ErrorCode(errFromAPI) == errors.CodeNotFound || errors.ErrorCode(errFromAPI) == errors.CodeUnauthorized)
	if modelDeleted {
		if err := j.deleteModel(ctx, m.ResourceTag()); err != nil {
			return errors.E(op, fmt.Errorf("failed to delete model: %w", err))
		}
	}
	return nil
}
