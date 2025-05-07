// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/errors"
)

// ControllerConfig retrieves the controller configuration.
func (c Connection) ControllerConfig(ctx context.Context) (jujuparams.ControllerConfigResult, error) {
	const op = errors.Op("jujuclient.ControllerConfig")
	results := jujuparams.ControllerConfigResult{}
	if err := c.CallHighestFacadeVersion(ctx, "Controller", []int{12}, "", "ControllerConfig", nil, &results); err != nil {
		return jujuparams.ControllerConfigResult{}, errors.E(op, jujuerrors.Cause(err))
	}
	if results.Config == nil {
		return jujuparams.ControllerConfigResult{}, errors.E(op, errors.CodeNotFound, "controller config not found")
	}
	return results, nil
}
