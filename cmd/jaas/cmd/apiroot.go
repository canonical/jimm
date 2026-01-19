package cmd

import (
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// APIClientFunc is a function that returns a JIMMAPI client.
type APIClientFunc func(dialOpts *jujuapi.DialOpts) (JIMMAPI, error)

// NewClient creates a new JIMMAPI client using the provided dial options.
func NewClient(dialOpts *jujuapi.DialOpts) (JIMMAPI, error) {
	store := jujuclient.NewFileClientStore()
	currentController, err := store.CurrentController()
	if err != nil {
		return nil, errors.E(err, "could not determine the current controller")
	}

	modelCmd := modelcmd.ControllerCommandBase{}
	apiCaller, err := modelCmd.NewAPIRootWithDialOpts(store, currentController, "", dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
