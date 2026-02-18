// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	"github.com/juju/juju/api/client/client"
	jujuparams "github.com/juju/juju/rpc/params"
)

// Status returns the status of the juju model.
func (c Connection) Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error) {
	return client.NewClient(&c, nil).Status(&client.StatusArgs{
		Patterns: patterns,
	})
}
