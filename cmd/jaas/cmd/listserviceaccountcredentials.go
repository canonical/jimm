// Copyright 2021 Canonical Ltd.

package cmd

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

var (
	listServiceCredentialsCommandDoc = `
list-credentials lists the cloud credentials belonging to a service account.

This command only shows credentials uploaded to the controller that belong to the service account.
Client-side credentials should be managed via the juju credentials command.

`
	listServiceAccountCredentialsExamples = `
    juju list-service-account-credentials <client-id> 
    juju list-service-account-credentials <client-id> --show-secrets
    juju list-service-account-credentials <client-id> --format yaml
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
		Name:     "list-service-account-credentials",
		Purpose:  "List service account cloud credentials",
		Args:     "<client-id>",
		Doc:      listServiceCredentialsCommandDoc,
		Examples: listServiceAccountCredentialsExamples,
	})
}

// SetFlags implements Command.SetFlags.
func (c *listServiceAccountCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatCredentialsTabular,
	})
	f.BoolVar(&c.showSecrets, "show-secrets", false, "Show secrets, applicable to yaml or json formats only")
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
	ServiceAccount map[string]cloud.CloudCredential `yaml:"controller-credentials,omitempty" json:"controller-credentials,omitempty"`
}

// Run implements Command.Run.
func (c *listServiceAccountCredentialsCommand) Run(ctxt *cmd.Context) error {
	if c.showSecrets && c.out.Name() == "tabular" {
		ctxt.Infof("secrets are not shown in tabular format")
		c.showSecrets = false
	}

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
		CloudCredentialArgs: params.CloudCredentialArgs{IncludeSecrets: c.showSecrets},
	}
	client := api.NewClient(apiCaller)
	resp, err := client.ListServiceAccountCredentials(&params)
	if err != nil {
		return errors.E(err)
	}
	svcAccCreds := credentialsMap{ServiceAccount: credentialMapByCloud(*ctxt, resp.Results)}

	err = c.out.Write(ctxt, svcAccCreds)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func credentialMapByCloud(ctxt cmd.Context, credentials []params.CredentialContentResult) map[string]cloud.CloudCredential {
	byCloud := make(map[string]cloud.CloudCredential)
	for _, credential := range credentials {
		if credential.Error != nil {
			ctxt.Warningf("error loading remote credential: %v", credential.Error)
			continue
		}
		remoteCredential := credential.Result.Content
		cloudCredential := byCloud[remoteCredential.Cloud]
		if cloudCredential.Credentials == nil {
			cloudCredential.Credentials = make(map[string]cloud.Credential)
		}
		cloudCredential.Credentials[remoteCredential.Name] = cloud.Credential{AuthType: remoteCredential.AuthType, Attributes: remoteCredential.Attributes}
		byCloud[remoteCredential.Cloud] = cloudCredential
	}
	return byCloud
}

// formatCredentialsTabular writes a tabular summary of cloud information.
// Adapted from juju/cmd/juju/cloud/listcredentials.go
func formatCredentialsTabular(writer io.Writer, value interface{}) error {
	credentials, ok := value.(credentialsMap)
	if !ok {
		return errors.E(fmt.Sprintf("expected value of type %T, got %T", credentials, value))
	}

	if len(credentials.ServiceAccount) == 0 {
		return nil
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}
	w.SetColumnAlignRight(1)

	printGroup := func(group map[string]cloud.CloudCredential) {
		w.Println("Cloud", "Credentials")
		// Sort alphabetically by cloud, and then by credential name.
		var cloudNames []string
		for name := range group {
			cloudNames = append(cloudNames, name)
		}
		slices.Sort(cloudNames)

		for _, cloudName := range cloudNames {
			var haveDefault bool
			var credentialNames []string
			credentials := group[cloudName]
			for credentialName := range credentials.Credentials {
				if credentialName == credentials.DefaultCredential {
					credentialNames = append([]string{credentialName + "*"}, credentialNames...)
					haveDefault = true
				} else {
					credentialNames = append(credentialNames, credentialName)
				}
			}
			if len(credentialNames) == 0 {
				w.Println(fmt.Sprintf("No credentials to display for cloud %v", cloudName))
				continue
			}
			if haveDefault {
				slices.Sort(credentialNames[1:])
			} else {
				slices.Sort(credentialNames)
			}
			w.Println(cloudName, strings.Join(credentialNames, ", "))
		}
	}
	w.Println("\nController Credentials:")
	printGroup(credentials.ServiceAccount)

	tw.Flush()
	return nil
}
