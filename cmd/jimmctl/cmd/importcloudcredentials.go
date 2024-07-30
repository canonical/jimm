// Copyright 2021 Canonical Ltd.

package cmd

import (
	"encoding/json"

	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
)

const importCloudCredentialsDoc = `
	import-cloud-credentials imports a set of cloud credentials
	loaded from a file containing a series of JSON objects. The JSON
	objects specifying the credentials should be of the form:

	{
		"_id": <cloud-credential-id>,
		"type": <credential-type>,
		"attributes": {
			<key1>: <value1>,
			...
		}
	}

	Example:
		jimmctl import-cloud-credentials creds.json
`

// NewImportCloudCredentialsCommand returns a command to import cloud
// credentials.
func NewImportCloudCredentialsCommand() cmd.Command {
	cmd := &importCloudCredentialsCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.file.StdinMarkers = stdinMarkers
	return modelcmd.WrapBase(cmd)
}

// importCloudCredentialsCommand imports a set of cloud credentials.
type importCloudCredentialsCommand struct {
	modelcmd.ControllerCommandBase
	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	file cmd.FileVar
}

// Info implements cmd.Command interface.
func (c *importCloudCredentialsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "import-cloud-credentials",
		Args:    "<credentials file>",
		Purpose: "Import cloud credentials to jimm",
		Doc:     importCloudCredentialsDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *importCloudCredentialsCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("filename not specified")
	}
	c.file.Path = args[0]
	if len(args) > 1 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements the cmd.Command interface.
func (c *importCloudCredentialsCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := cloud.NewClient(apiCaller)

	rc, err := c.file.Open(ctxt)
	if err != nil {
		return errors.E(err)
	}
	defer rc.Close()

	d := json.NewDecoder(rc)
	for d.More() {
		var cred credential
		if err := d.Decode(&cred); err != nil {
			return errors.E(err)
		}
		ctxt.Verbosef("importing %s", cred.Tag().Id())
		resp, err := client.AddCloudsCredentials(map[string]jujucloud.Credential{
			cred.Tag().String(): cred.Credential(),
		})
		if err == nil && resp[0].Error != nil {
			err = resp[0].Error
		}
		if err != nil {
			ctxt.Warningf("failed adding credential %s: %s", cred.Tag().Id(), err)
		}
	}
	return nil
}

type credential struct {
	ID         string            `json:"_id"`
	Type       string            `json:"type"`
	Attributes map[string]string `json:"attributes"`
}

// Tag returns the names.Tag for the credential.
func (c credential) Tag() names.Tag {
	tag := names.NewCloudCredentialTag(c.ID)
	if tag.Owner().IsLocal() {
		id := tag.Cloud().Id() + "/" + tag.Owner().WithDomain("external").Id() + "/" + tag.Name()
		tag = names.NewCloudCredentialTag(id)
	}
	return tag
}

// Credential returns the jujucloud.Credential for this credential.
func (c credential) Credential() jujucloud.Credential {
	return jujucloud.NewCredential(jujucloud.AuthType(c.Type), c.Attributes)
}
