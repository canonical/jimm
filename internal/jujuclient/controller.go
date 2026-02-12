// Copyright 2026 Canonical.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
)

// ControllerConfig retrieves the controller configuration.
func (c Connection) ControllerConfig(ctx context.Context) (jujuparams.ControllerConfigResult, error) {

	results := jujuparams.ControllerConfigResult{}
	if err := c.CallHighestFacadeVersion(ctx, "Controller", []int{12}, "", "ControllerConfig", nil, &results); err != nil {
		return jujuparams.ControllerConfigResult{}, errors.E(jujuerrors.Cause(err))
	}
	if results.Config == nil {
		return jujuparams.ControllerConfigResult{}, errors.E(errors.CodeNotFound, "controller config not found")
	}
	return results, nil
}

// CloudSpec retrieves the cloud spec of the model connected to.
func (c Connection) CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error) {
	modelCfgClient := modelconfig.NewClient(&c)
	attrs, err := modelCfgClient.ModelGet()
	if err != nil {
		return cloudspec.CloudSpec{}, err
	}

	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return cloudspec.CloudSpec{}, err
	}

	controllerClient := controller.NewClient(&c)
	return controllerClient.CloudSpec(names.NewModelTag(cfg.UUID()))
}
