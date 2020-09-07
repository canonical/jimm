// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/rogpeppe/fastuuid"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
	jimmversion "github.com/CanonicalLtd/jimm/version"
)

func init() {
	facadeInit["Controller"] = func(r *controllerRoot) []int {
		allModelsMethod := rpc.Method(r.AllModels)
		configSetMethod := rpc.Method(r.ConfigSet)
		controllerConfigMethod := rpc.Method(r.ControllerConfig)
		controllerVersionMethod := rpc.Method(r.ControllerVersion)
		identityProviderURLMethod := rpc.Method(r.IdentityProviderURL)
		modelConfigMethod := rpc.Method(r.ModelConfig)
		modelStatusMethod := rpc.Method(r.ModelStatus)
		mongoVersionMethod := rpc.Method(r.MongoVersion)
		watchModelSummariesMethod := rpc.Method(r.WatchModelSummaries)

		r.AddMethod("Controller", 3, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 3, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 3, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 3, "ModelStatus", modelStatusMethod)

		r.AddMethod("Controller", 4, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 4, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 4, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 4, "ModelStatus", modelStatusMethod)

		r.AddMethod("Controller", 5, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 5, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 5, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 5, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 5, "ModelStatus", modelStatusMethod)

		r.AddMethod("Controller", 6, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 6, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 6, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 6, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 6, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 6, "MongoVersion", mongoVersionMethod)

		r.AddMethod("Controller", 7, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 7, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 7, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 7, "IdentityProviderURL", identityProviderURLMethod)
		r.AddMethod("Controller", 7, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 7, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 7, "MongoVersion", mongoVersionMethod)

		r.AddMethod("Controller", 8, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 8, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 8, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 8, "ControllerVersion", controllerVersionMethod)
		r.AddMethod("Controller", 8, "IdentityProviderURL", identityProviderURLMethod)
		r.AddMethod("Controller", 8, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 8, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 8, "MongoVersion", mongoVersionMethod)

		r.AddMethod("Controller", 9, "AllModels", allModelsMethod)
		r.AddMethod("Controller", 9, "ConfigSet", configSetMethod)
		r.AddMethod("Controller", 9, "ControllerConfig", controllerConfigMethod)
		r.AddMethod("Controller", 9, "ControllerVersion", controllerVersionMethod)
		r.AddMethod("Controller", 9, "IdentityProviderURL", identityProviderURLMethod)
		r.AddMethod("Controller", 9, "ModelConfig", modelConfigMethod)
		r.AddMethod("Controller", 9, "ModelStatus", modelStatusMethod)
		r.AddMethod("Controller", 9, "MongoVersion", mongoVersionMethod)
		r.AddMethod("Controller", 9, "WatchModelSummaries", watchModelSummariesMethod)

		return []int{3, 4, 5, 6, 7, 8, 9}
	}
}

// ConfigSet changes the value of specified controller configuration
// settings. Only some settings can be changed after bootstrap.
// JIMM does not support changing settings via ConfigSet.
func (r *controllerRoot) ConfigSet(ctx context.Context, args jujuparams.ControllerConfigSet) error {
	return nil
}

// MongoVersion allows the introspection of the mongo version per controller.
func (r *controllerRoot) MongoVersion(ctx context.Context) (jujuparams.StringResult, error) {
	return r.jem.MongoVersion(ctx)
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
	srvVersion, err := r.jem.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.ControllerVersionResults{}, errgo.Mask(err)
	}
	result := jujuparams.ControllerVersionResults{
		Version:   srvVersion.String(),
		GitCommit: jimmversion.VersionInfo.GitCommit,
	}
	return result, nil
}

func (r *controllerRoot) WatchModelSummaries(ctx context.Context) (jujuparams.SummaryWatcherID, error) {
	// TODO(mhilton) move this somewhere where it will be reused accross connections
	r.mu.Lock()
	if r.generator == nil {
		var err error
		r.generator, err = fastuuid.NewGenerator()
		r.mu.Unlock()
		if err != nil {
			return jujuparams.SummaryWatcherID{}, errgo.Mask(err)
		}
	} else {
		r.mu.Unlock()
	}

	id := fmt.Sprintf("%v", r.generator.Next())

	watcher, err := newModelSummaryWatcher(auth.ContextWithIdentity(ctx, r.identity), id, r, r.jem.Pubsub())
	if err != nil {
		return jujuparams.SummaryWatcherID{}, errgo.Mask(err)
	}
	r.watchers.register(watcher)

	return jujuparams.SummaryWatcherID{
		WatcherID: id,
	}, nil
}

func (r *controllerRoot) AllModels(ctx context.Context) (jujuparams.UserModelList, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	return r.allModels(ctx)
}

// allModels returns all the models the logged in user has access to.
func (r *controllerRoot) allModels(ctx context.Context) (jujuparams.UserModelList, error) {
	var models []jujuparams.UserModel
	err := r.doModels(ctx, func(ctx context.Context, model *mongodoc.Model) error {
		models = append(models, jujuparams.UserModel{
			Model:          userModelForModelDoc(model),
			LastConnection: nil, // TODO (mhilton) work out how to record and set this.
		})
		return nil
	})
	if err != nil {
		return jujuparams.UserModelList{}, errgo.Mask(err)
	}
	return jujuparams.UserModelList{
		UserModels: models,
	}, nil
}

func (r *controllerRoot) ModelStatus(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelStatusResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	results := make([]jujuparams.ModelStatus, len(args.Entities))
	// TODO (fabricematrat) get status for all of the models connected
	// to a single controller in one go.
	for i, arg := range args.Entities {
		mi, err := r.modelStatus(ctx, arg)
		if err != nil {
			return jujuparams.ModelStatusResults{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		results[i] = *mi
	}

	return jujuparams.ModelStatusResults{
		Results: results,
	}, nil
}

// modelStatus retrieves the model status for the specified entity.
func (r *controllerRoot) modelStatus(ctx context.Context, arg jujuparams.Entity) (*jujuparams.ModelStatus, error) {
	mi, err := r.modelInfo(ctx, arg, false)
	if err != nil {
		return &jujuparams.ModelStatus{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &jujuparams.ModelStatus{
		ModelTag:           names.NewModelTag(mi.UUID).String(),
		Life:               mi.Life,
		HostedMachineCount: len(mi.Machines),
		ApplicationCount:   0,
		OwnerTag:           mi.OwnerTag,
		Machines:           mi.Machines,
	}, nil
}

// ControllerConfig returns the controller's configuration.
func (r *controllerRoot) ControllerConfig() (jujuparams.ControllerConfigResult, error) {
	result := jujuparams.ControllerConfigResult{
		Config: map[string]interface{}{
			"charmstore-url": r.params.CharmstoreLocation,
			"metering-url":   r.params.MeteringLocation,
		},
	}
	return result, nil
}

// ModelConfig returns implements the controller facade's ModelConfig
// method. This always returns a permission error, as no user has admin
// access to the controller.
func (r *controllerRoot) ModelConfig() (jujuparams.ModelConfigResults, error) {
	return jujuparams.ModelConfigResults{}, &jujuparams.Error{
		Code:    jujuparams.CodeUnauthorized,
		Message: "permission denied",
	}
}
