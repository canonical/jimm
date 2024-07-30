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
	"github.com/juju/names/v5"

	"github.com/juju/juju/rpc/params"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

var (
	updateCredentialCommandDoc = `
update-service-account-credential command updates the credentials associated with a service account.
Without any additional flags this command will search for the specified credentials on the controller
and create a copy that belongs to the service account.

If the --client option is provided, the command will search for the specified credential on your local
client store and upload a copy of the credential that will be owned by the service account.
`

	updateCredentialCommandExamples = `
    juju update-service-account-credential <client-id> aws <credential-name>
	juju update-service-account-credential --client <client-id> aws <credential-name> 

`
)

// NewUpdateCredentialCommand returns a command to update a service account's cloud credentials.
func NewUpdateCredentialCommand() cmd.Command {
	cmd := &updateCredentialCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// updateCredentialCommand updates a service account's cloud credentials.
type updateCredentialCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	clientID       string
	cloud          string
	credentialName string
	client         bool
}

// Info implements Command.Info.
func (c *updateCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-service-account-credential",
		Purpose:  "Update service account cloud credential",
		Args:     "<client-id> <cloud> <credential-name>",
		Doc:      updateCredentialCommandDoc,
		Examples: updateCredentialCommandExamples,
	})
}

// SetFlags implements Command.SetFlags.
func (c *updateCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.client, "client", false, "Provide this option to use a credential from your local store instead")
}

// Init implements the cmd.Command interface.
func (c *updateCredentialCommand) Init(args []string) error {
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
func (c *updateCredentialCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return errors.E(err, "failed to dial the controller")
	}
	var resp any
	if c.client {
		resp, err = c.updateFromLocalStore(apiCaller)
	} else {
		resp, err = c.updateFromControllerStore(apiCaller)
	}
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func (c *updateCredentialCommand) updateFromLocalStore(apiCaller jujuapi.Connection) (any, error) {
	credential, err := findCredentialsInLocalCache(c.store, c.cloud, c.credentialName)
	if err != nil {
		return nil, errors.E(err)
	}

	// Note that ensuring a client ID comes with the correct domain (which is
	// `@serviceaccount`) is not the responsibility of the CLI commands and is
	// actually taken care of in the `jujuapi` package. But, here, since we need
	// to create cloud credential tags, which are meant to be used by JIMM
	// internals, we have to make sure they're in the correct format.
	clientIdWithDomain, err := jimmnames.EnsureValidServiceAccountId(c.clientID)
	if err != nil {
		return nil, errors.E("invalid client ID")
	}

	taggedCredential := jujuparams.TaggedCredential{
		Tag:        names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", c.cloud, clientIdWithDomain, c.credentialName)).String(),
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
		return nil, errors.E(err)
	}
	return resp, nil
}

func (c *updateCredentialCommand) updateFromControllerStore(apiCaller jujuapi.Connection) (any, error) {
	params := apiparams.CopyServiceAccountCredentialRequest{
		ClientID:           c.clientID,
		CloudCredentialArg: params.CloudCredentialArg{CloudName: c.cloud, CredentialName: c.credentialName},
	}
	client := api.NewClient(apiCaller)
	res, err := client.CopyServiceAccountCredential(&params)
	if err != nil {
		return nil, errors.E(err)
	}
	return res, nil
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
