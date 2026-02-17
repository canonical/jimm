// Copyright 2026 Canonical.

package jujuclient

import (
	"context"

	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/controller/controller"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v5"
)

// ControllerConfig retrieves the controller configuration.
func (c Connection) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	return controller.NewClient(&c).ControllerConfig()
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
