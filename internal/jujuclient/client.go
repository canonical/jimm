// Copyright 2021 Canonical Ltd.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// Status returns the status of the juju model.
func (c Connection) Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error) {
	const op = errors.Op("jujuclient.Status")

	p := jujuparams.StatusParams{
		Patterns: patterns,
	}

	out := jujuparams.FullStatus{}
	if err := c.client.Call(ctx, "Client", 3, "", "FullStatus", &p, &out); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}

	return &out, nil
}
