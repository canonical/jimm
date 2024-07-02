// Copyright 2024 Canonical Ltd.

package jujuclient

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"
)

// AllModels allows controller administrators to get the list of all the
// models in the controller.
func (c Connection) AllModels(ctx context.Context) (jujuparams.UserModelList, error) {
	const op = errors.Op("jujuclient.AllModels")

	var resp jujuparams.UserModelList
	err := c.Call(ctx, "Controller", 11, "", "AllModels", nil, &resp)
	if err != nil {
		return resp, errors.E(op, jujuerrors.Cause(err))
	}
	return resp, nil
}
