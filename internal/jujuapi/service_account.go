// Copyright 2024 canonical.

package jujuapi

import (
	"context"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

// service_acount contains the primary RPC commands for handling service accounts within JIMM via the JIMM facade itself.

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddServiceAccount(ctx context.Context, req apiparams.AddServiceAccountRequest) error {
	const op = errors.Op("jujuapi.AddGroup")

	if !jimmnames.IsValidServiceAccountId(req.ID) {
		return errors.E(op, errors.CodeBadRequest, "invalid client ID")
	}

	return r.jimm.AddServiceAccount(ctx, r.user, req.ID)
}
