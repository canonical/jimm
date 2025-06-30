// Copyright 2025 Canonical.

package cmd

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/controller"
	controllerapi "github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/internal/errors"
	jimmapi "github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	migrateModelCommandDoc = `
The migrate command migrates a model to JIMM.

This command is useful to take a model that is already running on a Juju controller
and migrate it to JIMM. During this process JIMM will modify the details of the model
to remove any local users with access to the model and replace the model owner with
an external user i.e. from alice -> alice@canonical.com.

In order to determine the new model owner and to handle any existing application-offers 
that have already been consumed with local users, you can specify a user mapping file
with the --user-mapping flag. This should point to a yaml file with a mapping of local 
users to external users.
For example:

my-user-mapping.yaml:
'''
alice:alice@canonical.com
bob:bob@canonical.com
'''

The mapping must, at a minimum, contain an entry for the model owner.

The user mapping is consulted when relations are periodically validated. I.e. if an offer
was consumed by user "alice", when JIMM validates the relation it will understand that user
"alice" has been mapped and checks that "alice@canonical" still has access to the offer.
Revoking access from "alice@canonical.com" will result in the relation encountering an error.

It may not be possible to know all users that have have consumed offers from a model, so
the user mapping may need to be updated over time as offers fail due to permission errors.
Updating the user mapping is available from the "add-user-mapping" command.

Any tools/scripts that refer to models by their full name (owner/name) will need to be 
updated after migration to use the new external username or refer to models by their UUID.
`
	migrateModelCommandExample = `
    juju migrate alice/my-model target-controller --backing-controller=target-controller --user-mapping=./user-mapping.yaml
`
)

// NewMigrateModelCommand returns a command to migrate models.
func NewMigrateModelCommand() cmd.Command {
	cmd := &migrateModelCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// migrateModelCommand migrates a model to/from JAAS.
type migrateModelCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store             jujuclient.ClientStore
	dialOpts          *jujuapi.DialOpts
	targetController  string
	modelUUID         string
	backingController string
	userMappingFile   string
}

// Info implements Command.Info.
func (c *migrateModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "migrate",
		Args:     "<model-name> <target-controller-name>",
		Purpose:  "Migrate models to the JIMM targetting a specific controller managed by JIMM.",
		Doc:      migrateModelCommandDoc,
		Examples: migrateModelCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *migrateModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.backingController, "backing-controller", "", "Specify the name of the controller that will host the model in JIMM.")
	f.StringVar(&c.userMappingFile, "user-mapping", "", "Specify a comma-separated user mapping of local users to external users")
}

// Init implements the cmd.Command interface.
func (c *migrateModelCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.E("Missing controller name and model target arguments")
	}
	c.modelUUID = args[0]
	if !names.IsValidModel(c.modelUUID) {
		return errors.E(fmt.Errorf("Invalid model name %q", c.modelUUID))
	}
	c.targetController = args[1]
	if c.userMappingFile == "" {
		return errors.E("Missing user mapping file. Please provide a user mapping file with the --user-mapping flag.")
	}
	return nil
}

// Run implements Command.Run.
func (c *migrateModelCommand) Run(ctxt *cmd.Context) error {

	// Validate that the current controller exists.
	// This is the controller where the model currently resides.
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	// Contact the target controller (JIMM) to prepare it for migration
	// and receive a migration token.
	token, err := c.prepareMigration()
	if err != nil {
		return err
	}

	// Create the migration spec that will be used to initiate the migration.
	spec, err := c.getMigrationSpec(token)
	if err != nil {
		return err
	}

	// Dial the target controller and start the migration.
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := controllerapi.NewClient(apiCaller)
	events, err := client.InitiateMigration(spec)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, events)
	if err != nil {
		return err
	}
	return nil
}

func (c *migrateModelCommand) prepareMigration() (token []byte, err error) {
	userMapping, err := c.readUserMappingFile()
	if err != nil {
		return nil, err
	}
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, c.targetController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}
	client := jimmapi.NewClient(apiCaller)
	response, err := client.PrepareModelMigration(&apiparams.PrepareModelMigrationRequest{
		BackingControllerName: c.backingController,
		UserMapping:           userMapping,
		ModelTag:              names.NewModelTag(c.modelUUID).String(),
	})
	if err != nil {
		return nil, err
	}
	return response.Token, nil
}

func (c *migrateModelCommand) readUserMappingFile() (map[string]string, error) {
	content, err := os.ReadFile(c.userMappingFile)
	if err != nil {
		return nil, errors.E(err, "could not read user mapping file")
	}
	userMapping := make(map[string]string)
	err = yaml.Unmarshal(content, &userMapping)
	if err != nil {
		return nil, err
	}
	return userMapping, nil
}

func (c *migrateModelCommand) getMigrationSpec(token []byte) (controller.MigrationSpec, error) {
	store := c.store
	accountDetails, err := store.AccountDetails(c.targetController)
	if err != nil {
		return controllerapi.MigrationSpec{}, errors.E(err, "could not get account details for target controller")
	}

	controllerInfo, err := store.ControllerByName(c.targetController)
	if err != nil {
		return controllerapi.MigrationSpec{}, err
	}

	return controller.MigrationSpec{
		TargetControllerUUID:  controllerInfo.ControllerUUID,
		TargetControllerAlias: c.targetController,
		TargetAddrs:           controllerInfo.APIEndpoints,
		TargetCACert:          controllerInfo.CACert,
		ModelUUID:             c.modelUUID,
		TargetToken:           base64.StdEncoding.EncodeToString(token),
		// The target user is not needed here, as the user details will be determined
		// by the contents of the migration token - a JWT token parsed by JIMM.
		// But Juju requires this field to be set, so we provide the user running the migration.
		TargetUser: accountDetails.User,
	}, nil
}
