// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

var (
	listServiceCredentialsCommandDoc = `
list-credentials command list the cloud credentials belonging to a service account.

Example:
	juju service-account list-credentials <clientID> 
	juju service-account list-credentials <clientID> --show-secrets
	juju service-account list-credentials <clientID> --format yaml
`
)

// NewAddControllerCommand returns a command to add a service account
func NewListServiceAccountCredentialsCommand() cmd.Command {
	cmd := &listServiceAccountCredentialsCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// listServiceAccountCredentialsCommand binds a service account to a user.
type listServiceAccountCredentialsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store       jujuclient.ClientStore
	dialOpts    *jujuapi.DialOpts
	clientID    string
	showSecrets bool
}

func (c *listServiceAccountCredentialsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "list-credentials",
		Purpose: "List service account cloud credentials",
		Doc:     listServiceCredentialsCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *listServiceAccountCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.showSecrets, "show-secrets", false, "Show secrets")
}

// Init implements the cmd.Command interface.
func (c *listServiceAccountCredentialsCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("clientID not specified")
	}
	c.clientID = args[0]
	if len(args) > 1 {
		return errors.E("too many args")
	}
	return nil
}

type credentialsMap struct {
	// ServiceAccount has a collection of all ServiceAccount credentials keyed on credential name.
	ServiceAccount map[string]cloud.CloudCredential `yaml:"service-account-credentials,omitempty" json:"controller-credentials,omitempty"`
}

// Run implements Command.Run.
func (c *listServiceAccountCredentialsCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	params := apiparams.ListServiceAccountCredentialsRequest{
		ClientID:            c.clientID,
		CloudCredentialArgs: params.CloudCredentialArgs{},
	}
	client := api.NewClient(apiCaller)
	resp, err := client.ListServiceAccountCredentials(&params)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, credentialsByCloud(*ctxt, resp.Results))
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func credentialsByName(ctxt cmd.Context, credentials []params.CredentialContentResult) map[string]cloud.CloudCredential {
	byCloud := map[string]cloud.CloudCredential{}
	for _, credential := range credentials {
		if credential.Error != nil {
			ctxt.Warningf("error loading remote credential: %v", credential.Error)
			continue
		}
		remoteCredential := credential.Result.Content
		cloudCredential, ok := byCloud[remoteCredential.Cloud]
		if !ok {
			cloudCredential = cloud.CloudCredential{}
		}
		if cloudCredential.Credentials == nil {
			cloudCredential.Credentials = map[string]cloud.Credential{}
		}
		cloudCredential.Credentials[remoteCredential.Name] = cloud.Credential{AuthType: remoteCredential.AuthType, Attributes: remoteCredential.Attributes}
		byCloud[remoteCredential.Cloud] = cloudCredential
	}
	return byCloud
}
