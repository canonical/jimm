// Copyright 2024 Canonical.
package jujuclient2

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/modelmanager"
)

// attempt 1

type ModelManagerClientFactoryFunc func(DialParams) (ModelManagerClient, error)

func GetModelManagerFactory(d Dialer) ModelManagerClientFactoryFunc {
	return func(p DialParams) (ModelManagerClient, error) {
		conn, err := d.Dial(p)
		if err != nil {
			return nil, err
		}
		return modelmanager.NewClient(conn), nil
	}
}

// attempt 2

type clientFactory struct {
	d Dialer

	newModelManager func(api.Connection) ModelManagerClient
}

type Dialer interface {
	Dial(DialParams) (api.Connection, error)
}

type clientOption func(jjc *clientFactory)

func WithNewModelManager(mmc ModelManagerClient) clientOption {
	return func(jjc *clientFactory) {
		jjc.newModelManager = func(c api.Connection) ModelManagerClient {
			return mmc
		}
	}
}

func NewJujuClient(dialer Dialer, options ...clientOption) *clientFactory {
	jjc := &clientFactory{
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
func (c *clientFactory) ModelManager(p DialParams) (ModelManagerClient, error) {
	conn, err := c.d.Dial(p)
	if err != nil {
		return nil, err
	}

	return c.newModelManager(conn), nil
}
