// Copyright 2024 Canonical Ltd.

package cmd

import (
	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

var (
	addServiceAccountCredentialsCommandDoc = `
add-service-account-credentials copies a cloud-credentials on the controller belong to your user over to the service account.
This allows for an interactive way to set the cloud-credentials for a service account.

Note that you should first upload a set of cloud-credentials to the controller with juju add-credential.
`
	addServiceAccountCredentialsCommandExamples = `
    juju add-service-account-credentials <client-id> <cloud-name> <credential-name>
`
)

// NewAddServiceAccountCredentialCommand returns a command to copy a user cloud-credential to a service account.
func NewAddServiceAccountCredentialCommand() cmd.Command {
	cmd := &addServiceAccountCredentialCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addServiceAccountCredentialCommand holds the input and fields for executing the command.
type addServiceAccountCredentialCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store          jujuclient.ClientStore
	dialOpts       *jujuapi.DialOpts
	clientID       string
	cloud          string
	credentialName string
}

// Info implements Command.Info.
func (c *addServiceAccountCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-service-account",
		Purpose:  "Add permission to manage a service account",
		Args:     "<client-id>",
		Examples: addServiceAccountCredentialsCommandExamples,
		Doc:      addServiceAccountCredentialsCommandDoc,
	})
}

// SetFlags implements the cmd.Command interface.
func (c *addServiceAccountCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *addServiceAccountCredentialCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("client ID not specified")
	}
	c.clientID = args[0]
	if len(args) < 2 {
		return errors.E("cloud not specified")
	}
	c.cloud = args[1]
	if len(args) < 3 {
		return errors.E("credential name not specified")
	}
	c.credentialName = args[2]
	if len(args) > 3 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *addServiceAccountCredentialCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	params := apiparams.AddServiceAccountCredentialRequest{
		ClientID:           c.clientID,
		CloudCredentialArg: params.CloudCredentialArg{CloudName: c.cloud, CredentialName: c.credentialName},
	}
	client := api.NewClient(apiCaller)
	err = client.AddServiceAccountCredential(&params)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, "credential added successfully")
	if err != nil {
		return errors.E(err)
	}
	return nil
}
