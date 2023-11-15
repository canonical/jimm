// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jujuapi/rpc"
	"github.com/canonical/jimm/internal/openfga"
	jimmversion "github.com/canonical/jimm/version"
)

func init() {
	facadeInit["Controller"] = func(r *controllerRoot) []int {
		allModelsMethod := rpc.Method(r.AllModels)
		configSetMethod := rpc.Method(r.ConfigSet)
		controllerConfigMethod := rpc.Method(r.ControllerConfig)
		controllerVersionMethod := rpc.Method(r.ControllerVersion)
		getControllerAccessMethod := rpc.Method(r.GetControllerAccess)
		identityProviderURLMethod := rpc.Method(r.IdentityProviderURL)
		modelConfigMethod := rpc.Method(r.ModelConfig)
		modelStatusMethod := rpc.Method(r.ModelStatus)
		mongoVersionMethod := rpc.Method(r.MongoVersion)
		watchModelSummariesMethod := rpc.Method(r.WatchModelSummaries)
		watchAllModelSummariesMethod := rpc.Method(r.WatchAllModelSummaries)

		r.AddMethod("Controller", 11, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 11, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 11, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 11, "ControllerVersion", controllerVersionMethod)
		r.AddMethod("Controller", 11, "GetControllerAccess", getControllerAccessMethod)
		r.AddMethod("Controller", 11, "IdentityProviderURL", identityProviderURLMethod)
		r.AddMethod("Controller", 11, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 11, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 11, "MongoVersion", mongoVersionMethod)
		r.AddMethod("Controller", 11, "WatchModelSummaries", watchModelSummariesMethod)
		r.AddMethod("Controller", 11, "WatchAllModelSummaries", watchAllModelSummariesMethod)
		// TODO(Kian) Check missing methods
		// - DashboardConnectionInfo
		// - ListBlockedModels
		// - HostedModelConfig
		// - RemoveBlocks
		// - WatchAllModels
		// - initiateMigration
		// - ModifyControllerAccess
		// Methods that no longer exist:
		// - ControllerConfig
		// - ModelStatus

		return []int{11}
	}
}

// ConfigSet changes the value of specified controller configuration
// settings. Only some settings can be changed after bootstrap.
// JIMM does not support changing settings via ConfigSet.
func (r *controllerRoot) ConfigSet(ctx context.Context, args jujuparams.ControllerConfigSet) error {
	const op = errors.Op("jujuapi.ConfigSet")

	err := r.jimm.SetControllerConfig(ctx, r.user, args)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// MongoVersion allows the introspection of the mongo version per
// controller. This returns a not-supported error as JIMM does not use
// mongodb for a database.
func (r *controllerRoot) MongoVersion(ctx context.Context) (jujuparams.StringResult, error) {
	return jujuparams.StringResult{}, errors.E(errors.CodeNotSupported)
}

// IdentityProviderURL returns the URL of the configured external identity
// provider for this controller or an empty string if no external identity
// provider has been configured when the controller was bootstrapped.
func (r *controllerRoot) IdentityProviderURL(ctx context.Context) (jujuparams.StringResult, error) {
	return jujuparams.StringResult{
		Result: r.params.IdentityLocation,
	}, nil
}

// ControllerVersion returns the version information associated with this
// controller binary.
func (r *controllerRoot) ControllerVersion(ctx context.Context) (jujuparams.ControllerVersionResults, error) {
	const op = errors.Op("jujuapi.ControllerVersion")

	srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.ControllerVersionResults{}, errors.E(op, err)
	}
	result := jujuparams.ControllerVersionResults{
		Version:   srvVersion.String(),
		GitCommit: jimmversion.VersionInfo.GitCommit,
	}
	return result, nil
}

// WatchModelSummaries implements the WatchModelSummaries command on the
// Controller facade.
func (r *controllerRoot) WatchModelSummaries(ctx context.Context) (jujuparams.SummaryWatcherID, error) {
	const op = errors.Op("jujuapi.WatchModelSummaries")

	err := r.setupUUIDGenerator()
	if err != nil {
		return jujuparams.SummaryWatcherID{}, errors.E(op, err)
	}

	id := fmt.Sprintf("%v", r.generator.Next())

	getModels := func(ctx context.Context) ([]string, error) {
		models, err := r.allModels(ctx)
		if err != nil {
			return nil, errors.E(err)
		}
		modelUUIDs := make([]string, len(models.UserModels))
		for i, model := range models.UserModels {
			modelUUIDs[i] = model.UUID
		}
		return modelUUIDs, nil
	}
	watcher, err := newModelSummaryWatcher(ctx, id, r, r.jimm.Pubsub, getModels)
	if err != nil {
		return jujuparams.SummaryWatcherID{}, errors.E(op, err)
	}
	r.watchers.register(watcher)

	return jujuparams.SummaryWatcherID{
		WatcherID: id,
	}, nil
}

// WatchAllModelSummaries implements the WatchAllModelSummaries command on the
// Controller facade.
func (r *controllerRoot) WatchAllModelSummaries(ctx context.Context) (jujuparams.SummaryWatcherID, error) {
	const op = errors.Op("jujuapi.WatchAllModelSummaries")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to check for controller admin access", zap.Error(err))
		return jujuparams.SummaryWatcherID{}, errors.E(op, err)
	}
	if !isControllerAdmin {
		return jujuparams.SummaryWatcherID{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	err = r.setupUUIDGenerator()
	if err != nil {
		return jujuparams.SummaryWatcherID{}, errors.E(op, err)
	}

	id := fmt.Sprintf("%v", r.generator.Next())

	getAllModels := func(ctx context.Context) ([]string, error) {
		var modelUUIDs []string
		err := r.jimm.ForEachModel(ctx, r.user, func(m *dbmodel.Model, _ jujuparams.UserAccessPermission) error {
			modelUUIDs = append(modelUUIDs, m.UUID.String)
			return nil
		})
		if err != nil {
			return nil, errors.E(op, err)
		}
		return modelUUIDs, nil
	}

	watcher, err := newModelSummaryWatcher(ctx, id, r, r.jimm.Pubsub, getAllModels)
	if err != nil {
		return jujuparams.SummaryWatcherID{}, errors.E(op, err)
	}
	r.watchers.register(watcher)

	return jujuparams.SummaryWatcherID{
		WatcherID: id,
	}, nil
}

// AllModels implments the AllModels command on the Controller facade.
func (r *controllerRoot) AllModels(ctx context.Context) (jujuparams.UserModelList, error) {
	return r.allModels(ctx)
}

// allModels returns all the models the logged in user has access to.
func (r *controllerRoot) allModels(ctx context.Context) (jujuparams.UserModelList, error) {
	const op = errors.Op("jujuapi.AllModels")

	var models []jujuparams.UserModel
	err := r.jimm.ForEachUserModel(ctx, r.user, func(m *dbmodel.Model, _ jujuparams.UserAccessPermission) error {
		// TODO(Kian) CSS-6040 Refactor the below to use a better abstraction for Postgres/OpenFGA to Juju types.
		var um jujuparams.UserModel
		um.Model = m.ToJujuModel()
		models = append(models, um)
		return nil
	})
	if err != nil {
		return jujuparams.UserModelList{}, errors.E(op, err)
	}
	return jujuparams.UserModelList{
		UserModels: models,
	}, nil
}

// ModelStatus implements the ModelStatus command on the Controller facade.
func (r *controllerRoot) ModelStatus(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelStatusResults, error) {
	const op = errors.Op("jujuapi.ModelStatus")

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ModelStatus, len(args.Entities))
	for i, arg := range args.Entities {
		mt, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		status, err := r.jimm.ModelStatus(ctx, r.user, mt)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}
		results[i] = *status
	}
	return jujuparams.ModelStatusResults{
		Results: results,
	}, nil
}

// ControllerConfig returns the controller's configuration.
func (r *controllerRoot) ControllerConfig(ctx context.Context) (jujuparams.ControllerConfigResult, error) {
	const op = errors.Op("jujuapi.ControllerConfig")

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to check access rights", zap.Error(err))
		return jujuparams.ControllerConfigResult{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	if !isAdmin {
		isControllerAdmin, err := openfga.IsAdministrator(ctx, r.user, names.NewControllerTag(r.params.ControllerUUID))
		if err != nil {
			zapctx.Error(ctx, "failed to check access rights", zap.Error(err))
			return jujuparams.ControllerConfigResult{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
		}
		isAdmin = isControllerAdmin
	}
	if !isAdmin {
		return jujuparams.ControllerConfigResult{
			Config: jujuparams.ControllerConfig{},
		}, nil
	}

	cfg, err := r.jimm.GetControllerConfig(ctx, r.user.User)
	if err != nil {
		return jujuparams.ControllerConfigResult{}, errors.E(op, err)
	}
	result := jujuparams.ControllerConfigResult{
		Config: jujuparams.ControllerConfig(cfg.Config),
	}

	return result, nil
}

// ModelConfig returns implements the controller facade's ModelConfig
// method.
// Before:
//
//	If the user is a controller superuser then this returns a
//	not-supported error, otherwise it returns permission denied.
//
// Now:
//
//	This method returns a not-supported error.
func (r *controllerRoot) ModelConfig() (jujuparams.ModelConfigResults, error) {
	return jujuparams.ModelConfigResults{}, errors.E(errors.CodeNotSupported)
}

// GetControllerAccess returns the access level on the controller for
// users.
func (r *controllerRoot) GetControllerAccess(ctx context.Context, args jujuparams.Entities) (jujuparams.UserAccessResults, error) {
	const op = errors.Op("jujuapi.GetControllerAccess")

	results := make([]jujuparams.UserAccessResult, len(args.Entities))
	for i, arg := range args.Entities {
		tag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		access, err := r.jimm.GetJimmControllerAccess(ctx, r.user, tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}
		results[i].Result = &jujuparams.UserAccess{
			UserTag: tag.String(),
			Access:  access,
		}
	}

	return jujuparams.UserAccessResults{
		Results: results,
	}, nil
}
