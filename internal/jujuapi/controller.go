// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"fmt"

	jujucontroller "github.com/juju/juju/controller"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	"github.com/canonical/jimm/v3/internal/openfga"
	jimmversion "github.com/canonical/jimm/v3/version"
)

func init() {
	facadeInit["Controller"] = func(r *controllerRoot) []int {
		allModelsMethod := rpc.Method(r.AllModels)
		configSetMethod := rpc.Method(r.ConfigSet)
		controllerConfigMethod := rpc.Method(r.ControllerConfig)
		controllerVersionMethod := rpc.Method(r.ControllerVersion)
		getControllerAccessMethod := rpc.Method(r.GetControllerAccess)
		identityProviderURLMethod := rpc.Method(r.IdentityProviderURL)
		modelStatusMethod := rpc.Method(r.ModelStatus)
		mongoVersionMethod := rpc.Method(r.MongoVersion)
		watchModelSummariesMethod := rpc.Method(r.WatchModelSummaries)
		watchAllModelSummariesMethod := rpc.Method(r.WatchAllModelSummaries)
		initiateMigrationMethod := rpc.Method(r.InitiateMigration)

		r.AddMethod("Controller", 11, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 11, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 11, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 11, "ControllerVersion", controllerVersionMethod)
		r.AddMethod("Controller", 11, "GetControllerAccess", getControllerAccessMethod)
		r.AddMethod("Controller", 11, "IdentityProviderURL", identityProviderURLMethod)
		r.AddMethod("Controller", 11, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 11, "MongoVersion", mongoVersionMethod)
		r.AddMethod("Controller", 11, "WatchModelSummaries", watchModelSummariesMethod)
		r.AddMethod("Controller", 11, "WatchAllModelSummaries", watchAllModelSummariesMethod)
		r.AddMethod("Controller", 11, "InitiateMigration", initiateMigrationMethod)

		r.AddMethod("Controller", 12, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 12, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 12, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 12, "ControllerVersion", controllerVersionMethod)
		r.AddMethod("Controller", 12, "GetControllerAccess", getControllerAccessMethod)
		r.AddMethod("Controller", 12, "IdentityProviderURL", identityProviderURLMethod)
		r.AddMethod("Controller", 12, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 12, "MongoVersion", mongoVersionMethod)
		r.AddMethod("Controller", 12, "WatchModelSummaries", watchModelSummariesMethod)
		r.AddMethod("Controller", 12, "WatchAllModelSummaries", watchAllModelSummariesMethod)
		r.AddMethod("Controller", 12, "InitiateMigration", initiateMigrationMethod)

		return []int{11, 12}
	}
}

// ControllerService defines the methods used to manage controllers.
type ControllerService interface {
	AddController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error
	GetControllerBootstrap(ctx context.Context, name string) (*dbmodel.ControllerBootstrap, error)
	ControllerInfo(ctx context.Context, user *openfga.User, name string) (*dbmodel.Controller, error)
	EarliestControllerVersion(ctx context.Context) (version.Number, error)
	ListControllerBootstraps(ctx context.Context) ([]dbmodel.ControllerBootstrap, error)
	ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error)
	RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error
}

// ConfigSet changes the value of specified controller configuration
// settings. Only some settings can be changed after bootstrap.
// JIMM does not support changing settings via ConfigSet.
func (r *controllerRoot) ConfigSet(ctx context.Context, args jujuparams.ControllerConfigSet) error {
	return errors.Codef(errors.CodeNotSupported, "not supported")
}

// MongoVersion allows the introspection of the mongo version per
// controller. This returns a not-supported error as JIMM does not use
// mongodb for a database.
func (r *controllerRoot) MongoVersion(ctx context.Context) (jujuparams.StringResult, error) {
	return jujuparams.StringResult{}, errors.Codef(errors.CodeNotSupported, "not supported")
}

// IdentityProviderURL returns the URL of the configured external identity
// provider for this controller or an empty string if no external identity
// provider has been configured when the controller was bootstrapped.
func (r *controllerRoot) IdentityProviderURL(ctx context.Context) (jujuparams.StringResult, error) {
	return jujuparams.StringResult{Result: ""}, nil
}

// ControllerVersion returns the version information associated with this
// controller binary.
func (r *controllerRoot) ControllerVersion(ctx context.Context) (jujuparams.ControllerVersionResults, error) {

	srvVersion, err := r.jimm.JujuManager().EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.ControllerVersionResults{}, err
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

	err := r.setupUUIDGenerator()
	if err != nil {
		return jujuparams.SummaryWatcherID{}, err
	}

	id := fmt.Sprintf("%v", r.generator.Next())

	getModels := func(ctx context.Context) ([]string, error) {
		models, err := r.allModels(ctx)
		if err != nil {
			return nil, err
		}
		modelUUIDs := make([]string, len(models.UserModels))
		for i, model := range models.UserModels {
			modelUUIDs[i] = model.UUID
		}
		return modelUUIDs, nil
	}
	watcher, err := newModelSummaryWatcher(ctx, id, r.jimm.PubSubHub(), getModels)
	if err != nil {
		return jujuparams.SummaryWatcherID{}, err
	}
	r.watchers.register(watcher)

	return jujuparams.SummaryWatcherID{
		WatcherID: id,
	}, nil
}

// WatchAllModelSummaries implements the WatchAllModelSummaries command on the
// Controller facade.
func (r *controllerRoot) WatchAllModelSummaries(ctx context.Context) (jujuparams.SummaryWatcherID, error) {

	if !r.user.JimmAdmin {
		return jujuparams.SummaryWatcherID{}, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	err := r.setupUUIDGenerator()
	if err != nil {
		return jujuparams.SummaryWatcherID{}, err
	}

	id := fmt.Sprintf("%v", r.generator.Next())

	getAllModels := func(ctx context.Context) ([]string, error) {
		var modelUUIDs []string
		err := r.jimm.JujuManager().ForEachModel(ctx, r.user, func(m *dbmodel.Model, _ jujuparams.UserAccessPermission) error {
			modelUUIDs = append(modelUUIDs, m.UUID.String)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return modelUUIDs, nil
	}

	watcher, err := newModelSummaryWatcher(ctx, id, r.jimm.PubSubHub(), getAllModels)
	if err != nil {
		return jujuparams.SummaryWatcherID{}, err
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

	var models []jujuparams.UserModel
	err := r.jimm.JujuManager().ForEachUserModel(ctx, r.user, func(m *dbmodel.Model, _ string) error {
		// TODO(Kian) CSS-6040 Refactor the below to use a better abstraction for Postgres/OpenFGA to Juju types.
		var um jujuparams.UserModel
		um.Model = m.ToJujuModel()
		models = append(models, um)
		return nil
	})
	if err != nil {
		return jujuparams.UserModelList{}, err
	}
	return jujuparams.UserModelList{
		UserModels: models,
	}, nil
}

// ModelStatus implements the ModelStatus command on the Controller facade.
func (r *controllerRoot) ModelStatus(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelStatusResults, error) {

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ModelStatus, len(args.Entities))
	for i, arg := range args.Entities {
		mt, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results[i].Error = r.mapError(ctx, errors.Codef(errors.CodeBadRequest, "%w", err))
			continue
		}
		status, err := r.jimm.JujuManager().ModelStatus(ctx, r.user, mt)
		if err != nil {
			results[i].Error = r.mapError(ctx, err)
			continue
		}
		results[i] = toModelStatusParams(status)
	}
	return jujuparams.ModelStatusResults{
		Results: results,
	}, nil
}

// ControllerConfig returns the JIMM's controller configuration.
func (r *controllerRoot) ControllerConfig(ctx context.Context) (jujuparams.ControllerConfigResult, error) {

	config, err := r.jimm.ConfigManager().GetConfig()
	if err != nil {
		return jujuparams.ControllerConfigResult{}, err
	}
	cfg := make(map[string]any)
	cfg[jujucontroller.ControllerUUIDKey] = config.ControllerUUID
	cfg[jujucontroller.SSHServerPort] = config.SSHPort
	// TODO: update this to use the key coming from juju when we update the juju dependency.
	cfg["ssh-host-key"] = config.SSHPublicHostKey
	cfg[jujucontroller.PublicDNSAddress] = config.PublicDNSName
	return jujuparams.ControllerConfigResult{
		Config: cfg,
	}, nil
}

// GetControllerAccess returns the access level on the controller for
// users.
func (r *controllerRoot) GetControllerAccess(ctx context.Context, args jujuparams.Entities) (jujuparams.UserAccessResults, error) {

	results := make([]jujuparams.UserAccessResult, len(args.Entities))
	for i, arg := range args.Entities {
		tag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			results[i].Error = r.mapError(ctx, errors.Codef(errors.CodeBadRequest, "%w", err))
			continue
		}
		access, err := r.jimm.PermissionManager().GetJimmControllerAccess(ctx, r.user, tag)
		if err != nil {
			results[i].Error = r.mapError(ctx, err)
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

// InitiateMigration attempts to begin the migration of one or
// more models to other controllers.
func (r *controllerRoot) InitiateMigration(ctx context.Context, args jujuparams.InitiateMigrationArgs) (jujuparams.InitiateMigrationResults, error) {

	results := make([]jujuparams.InitiateMigrationResult, len(args.Specs))
	for i, spec := range args.Specs {
		result, err := r.jimm.JujuManager().InitiateMigration(ctx, r.user, spec)
		if err != nil {
			result.Error = r.mapError(ctx, err)
		}
		results[i] = result
	}

	return jujuparams.InitiateMigrationResults{
		Results: results,
	}, nil
}
