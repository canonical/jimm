// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/errors"
)

// Status returns the status of the juju model.
func (c Connection) Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error) {

	p := jujuparams.StatusParams{
		Patterns: patterns,
	}

	out := jujuparams.FullStatus{}
	if err := c.CallHighestFacadeVersion(ctx, "Client", []int{8}, "", "FullStatus", &p, &out); err != nil {
		return nil, errors.E(jujuerrors.Cause(err))
	}

	return &out, nil
}
