// Copyright 2024 Canonical Ltd.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v4"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

var (
	updateCredentialsCommandDoc = `
update-credentials command updates the credentials associated with a service account.
This will add the credentials to JAAS if they were not found.
`

	updateCredentialsCommandExamples = `
    juju update-service-account-credentials update-credentials 00000000-0000-0000-0000-000000000000 aws credential-name
`
)

// NewUpdateCredentialsCommand returns a command to update a service account's cloud credentials.
func NewUpdateCredentialsCommand() cmd.Command {
	cmd := &updateCredentialsCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// updateCredentialsCommand updates a service account's cloud credentials.
type updateCredentialsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	clientID       string
	cloud          string
	credentialName string
}

// Info implements Command.Info.
func (c *updateCredentialsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-service-account-credentials",
		Purpose:  "Update service account cloud credentials",
		Args:     "<client-id> <cloud> <credential-name>",
		Doc:      updateCredentialsCommandDoc,
		Examples: updateCredentialsCommandExamples,
	})
}

// SetFlags implements Command.SetFlags.
func (c *updateCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *updateCredentialsCommand) Init(args []string) error {
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
func (c *updateCredentialsCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return errors.E(err, "failed to dial the controller")
	}

	credential, err := findCredentialsInLocalCache(c.store, c.cloud, c.credentialName)
	if err != nil {
		return errors.E(err)
	}

	taggedCredential := jujuparams.TaggedCredential{
		Tag:        names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", c.cloud, c.clientID, c.credentialName)).String(),
		Credential: *credential,
	}

	params := apiparams.UpdateServiceAccountCredentialsRequest{
		ClientID: c.clientID,
		UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
			Credentials: []jujuparams.TaggedCredential{taggedCredential},
		},
	}

	client := api.NewClient(apiCaller)
	resp, err := client.UpdateServiceAccountCredentials(&params)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func findCredentialsInLocalCache(store jujuclient.ClientStore, cloud, credentialName string) (*jujuparams.CloudCredential, error) {
	cloudCredentials, err := store.CredentialForCloud(cloud)
	if err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to fetch local credentials for cloud %q", cloud))
	}

	for name, aCredential := range cloudCredentials.AuthCredentials {
		if name == credentialName {
			return &jujuparams.CloudCredential{
				AuthType:   string(aCredential.AuthType()),
				Attributes: aCredential.Attributes(),
			}, nil
		}
	}

	return nil, errors.E(fmt.Sprintf("credential %q not found on local client; run `juju add-credential <cloud> --client` to add the credential to Juju local store first", credentialName))
}
