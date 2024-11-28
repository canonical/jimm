// Copyright 2024 Canonical.
package jujuclient2

import (
	"context"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

type jujuClient struct {
	d Dialer

	newModelManager func(api.Connection) ModelManagerClient
}

type Dialer interface {
	Dial(ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (api.Connection, error)
}

type clientOption func(jjc *jujuClient)

func WithNewModelManager(mmc ModelManagerClient) clientOption {
	return func(jjc *jujuClient) {
		jjc.newModelManager = func(c api.Connection) ModelManagerClient {
			return mmc
		}
	}
}

func NewJujuClient(dialer Dialer, options ...clientOption) *jujuClient {
	jjc := &jujuClient{
		d: dialer,
		newModelManager: func(c api.Connection) ModelManagerClient {
			return modelmanager.NewClient(c)
		},
	}

	for _, opt := range options {
		opt(jjc)
	}

	return jjc
}

// ModelManager returns the ModelManager client.
func (c *jujuClient) ModelManager(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (ModelManagerClient, error) {
	conn, err := c.d.Dial(ctl, modelTag, nil)
	if err != nil {
		return nil, err
	}

	return c.newModelManager(conn), nil
}
