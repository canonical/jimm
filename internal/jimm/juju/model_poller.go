// Copyright 2025 Canonical.

package juju

import (
	"context"

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
func (j *JujuManager) PollModels(ctx context.Context) (err error) {
	const op = errors.Op("jimm.CleanupNotFoundModels")
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
			switch m.MigrationMode {
			case dbmodel.MigrationModeNone:
				j.checkModelExists(ctx, api, m)
			case dbmodel.MigrationModeMigrateInternal:
				j.checkModelMigratedInternal(ctx, api, m)
			}
		}
	}
	return nil
}

// checkModelMigratedInternal checks if the model has been migrated from
// one controller managed by JIMM to another controller managed by JIMM.
func (j *JujuManager) checkModelMigratedInternal(ctx context.Context, api API, m *dbmodel.Model) {
	const op = errors.Op("jimm.checkModelMoved")
	zapctx.Info(ctx, string(op))

	// Check if the model has completed a migration.
	// If modelInfo returns without an error, it definitely hasn't moved yet.
	modelInfo := &jujuparams.ModelInfo{UUID: m.UUID.String}
	err := api.ModelInfo(ctx, modelInfo)
	if err == nil {
		// If the migration end time is set, it means the model has
		// failed to migrate otherwise we'd expect a redirect error.
		if modelInfo.Migration.End != nil {
			m.ProcessFailedMigration()
			err = j.Database.UpdateModel(ctx, m)
			if err != nil {
				zapctx.Error(ctx, "failed to update model after failed migration", zap.String("model", m.UUID.String), zap.Error(err))
			}
		}
		return
	}

	// Expect a redirect error if the model successfully migrated.
	isRedirectErr := errors.ErrorCode(err) == params.CodeRedirect
	if !isRedirectErr {
		zapctx.Error(ctx, "failed to get model info", zap.String("model", m.UUID.String), zap.Error(err))
		return
	}

	// Parse the redirect error to get the new controller details.
	errInfo := errors.ErrorInfo(err)
	if errInfo == nil {
		zapctx.Error(ctx, "missing error info in redirect error", zap.String("model", m.UUID.String), zap.Error(err))
		return
	}

	var redirectInfo params.RedirectErrorInfo
	err = params.Error{Info: errInfo}.UnmarshalInfo(&redirectInfo)
	if err != nil {
		zapctx.Error(ctx, "cannot unmarshal redirect info for model", zap.String("model", m.UUID.String), zap.Error(err))
		return
	}

	// We expect this controller will be known to JIMM.
	controller := dbmodel.Controller{Name: redirectInfo.ControllerAlias}
	err = j.Database.GetController(ctx, &controller)
	if err != nil {
		zapctx.Error(ctx, "cannot get controller for model", zap.String("controllerAlias", redirectInfo.ControllerAlias), zap.String("model", m.UUID.String), zap.Error(err))
		return
	}

	m.ProcessSuccessfulInternalMigration(controller.ID)

	err = j.Database.UpdateModel(ctx, m)
	if err != nil {
		zapctx.Error(ctx, "failed to update model after migration", zap.String("model", m.UUID.String), zap.Error(err))
		return
	}
	zapctx.Info(ctx, "model successfully migrated to controller", zap.String("model", m.UUID.String), zap.String("controller_name", controller.Name))
}

// checkModelExists checks if the model exists on the controller.
// This performs eventual cleanup of models that have been deleted through
// the API (since the deletion of a model is not immediate) and handles
// cases where the model was deleted directly on the underlying controller.
func (j *JujuManager) checkModelExists(ctx context.Context, api API, m *dbmodel.Model) {
	err := api.ModelInfo(ctx, &jujuparams.ModelInfo{UUID: m.UUID.String})
	if err == nil {
		// If the call succeeds, the model exists and we can return.
		return
	}
	// Some versions of juju return unauthorized for models that cannot be found.
	modelDeleted := (errors.ErrorCode(err) == errors.CodeNotFound || errors.ErrorCode(err) == errors.CodeUnauthorized)
	if modelDeleted {
		if err := j.deleteModel(ctx, m.ResourceTag()); err != nil {
			zapctx.Error(ctx, "failed to delete model", zap.String("model", m.UUID.String), zap.Error(err))
		}
	} else {
		zapctx.Error(ctx, "failed to get ModelInfo", zap.String("model", m.UUID.String), zap.Error(err))
	}
}
