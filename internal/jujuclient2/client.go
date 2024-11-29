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
type ClientFactory struct {
	d Dialer

	newModelManager func(api.Connection) ModelManagerClient
}

type Dialer interface {
	Dial(DialParams) (api.Connection, error)
}

type clientOption func(jjc *ClientFactory)

func WithNewModelManager(mmc ModelManagerClient) clientOption {
	return func(jjc *ClientFactory) {
		jjc.newModelManager = func(c api.Connection) ModelManagerClient {
			return mmc
		}
	}
}

func NewJujuClientFactory(dialer Dialer, options ...clientOption) *ClientFactory {
	jjc := &ClientFactory{
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
func (c *ClientFactory) ModelManager(p DialParams) (ModelManagerClient, error) {
	conn, err := c.d.Dial(p)
	if err != nil {
		return nil, err
	}

	return c.newModelManager(conn), nil
}

// type modelmanagerClient = modelmanager.Client
// type applicationoffersClient = applicationoffers.Client

// attempt 3 (flatten all clients together)

// type ClientFactory struct {
// 	d cacheJujuDialer
// }

// func (c *ClientFactory) Connect(params DialParams) (*JClient, error) {
// 	conn, err := c.d.Dial(params)
// 	if err != nil {
// 		return nil, err
// 	}

// 	j := JClient{
// 		*modelmanager.NewClient(conn),
// 		*applicationoffers.NewClient(conn),
// 	}

// 	return &j, nil
// }

// type JClient struct {
// 	modelmanagerClient
// 	applicationoffersClient
// }
