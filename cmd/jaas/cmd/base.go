// Copyright 2026 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/juju/cmd/modelcmd"

	"github.com/canonical/jimm/v3/pkg/api"
)

type jaasCommandBase struct {
	modelcmd.ControllerCommandBase
	jimmAPI JIMMAPI
}

func (c *jaasCommandBase) setJIMMAPI(api JIMMAPI) {
	c.jimmAPI = api
}

func (c *jaasCommandBase) getJIMMAPI() (JIMMAPI, error) {
	return c.getJIMMAPIWithController("")
}

func (c *jaasCommandBase) getJIMMAPIWithController(controller string) (JIMMAPI, error) {
	if c.jimmAPI != nil {
		return c.jimmAPI, nil
	}

	currentController := controller
	if currentController == "" {
		var err error
		currentController, err = c.ClientStore().CurrentController()
		if err != nil {
			return nil, fmt.Errorf("could not determine controller: %w", err)
		}
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
