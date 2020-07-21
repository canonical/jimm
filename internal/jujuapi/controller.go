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
	"github.com/CanonicalLtd/jimm/params"
	jimmversion "github.com/CanonicalLtd/jimm/version"
)

type controllerV3 struct {
	*controllerRoot
}

type controllerV4 struct {
	*controllerV3
}

type controllerV5 struct {
	*controllerV4
}

// ConfigSet changes the value of specified controller configuration
// settings. Only some settings can be changed after bootstrap.
// JIMM does not support changing settings via ConfigSet.
func (c controllerV5) ConfigSet(ctx context.Context, args jujuparams.ControllerConfigSet) error {
	return nil
}

type controllerV6 struct {
	*controllerV5
}

// MongoVersion allows the introspection of the mongo version per controller.
func (c controllerV6) MongoVersion(ctx context.Context) (jujuparams.StringResult, error) {
	return c.jem.MongoVersion(ctx)
}

type controllerV7 struct {
	*controllerV6
}

// IdentityProviderURL returns the URL of the configured external identity
// provider for this controller or an empty string if no external identity
// provider has been configured when the controller was bootstrapped.
func (c controllerV7) IdentityProviderURL(ctx context.Context) (jujuparams.StringResult, error) {
	return jujuparams.StringResult{
		Result: c.params.IdentityLocation,
	}, nil
}

type controllerV8 struct {
	*controllerV7
}

// ControllerVersion returns the version information associated with this
// controller binary.
func (c controllerV8) ControllerVersion(ctx context.Context) (jujuparams.ControllerVersionResults, error) {
	srvVersion, err := c.jem.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.ControllerVersionResults{}, errgo.Mask(err)
	}
	result := jujuparams.ControllerVersionResults{
		Version:   srvVersion.String(),
		GitCommit: jimmversion.VersionInfo.GitCommit,
	}
	return result, nil
}

type controllerV9 struct {
	*controllerV8

	generator *fastuuid.Generator
}

func (c controllerV9) WatchModelSummaries(ctx context.Context) (jujuparams.SummaryWatcherID, error) {
	id := fmt.Sprintf("%v", c.generator.Next())

	watcher, err := newModelSummaryWatcher(auth.ContextWithIdentity(ctx, c.identity), id, c.controllerRoot, c.jem.Pubsub())
	if err != nil {
		return jujuparams.SummaryWatcherID{}, errgo.Mask(err)
	}
	c.watchers.register(watcher)

	return jujuparams.SummaryWatcherID{
		WatcherID: id,
	}, nil
}

func (c *controllerV3) AllModels(ctx context.Context) (jujuparams.UserModelList, error) {
	ctx = auth.ContextWithIdentity(ctx, c.identity)
	return c.allModels(ctx)
}

func (c *controllerV3) ModelStatus(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelStatusResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = auth.ContextWithIdentity(ctx, c.identity)
	results := make([]jujuparams.ModelStatus, len(args.Entities))
	// TODO (fabricematrat) get status for all of the models connected
	// to a single controller in one go.
	for i, arg := range args.Entities {
		mi, err := c.modelStatus(ctx, arg)
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
func (c *controllerV3) modelStatus(ctx context.Context, arg jujuparams.Entity) (*jujuparams.ModelStatus, error) {
	mi, err := c.modelInfo(ctx, arg, false)
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
func (c *controllerV3) ControllerConfig() (jujuparams.ControllerConfigResult, error) {
	result := jujuparams.ControllerConfigResult{
		Config: map[string]interface{}{
			"charmstore-url": c.params.CharmstoreLocation,
			"metering-url":   c.params.MeteringLocation,
		},
	}
	return result, nil
}

// ModelConfig returns implements the controller facade's ModelConfig
// method. This always returns a permission error, as no user has admin
// access to the controller.
func (c *controllerV3) ModelConfig() (jujuparams.ModelConfigResults, error) {
	return jujuparams.ModelConfigResults{}, &jujuparams.Error{
		Code:    jujuparams.CodeUnauthorized,
		Message: "permission denied",
	}
}
