// Copyright 2025 Canonical.

package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/api/client/modelmanager"
	controllerapi "github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v3"

	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmAPI "github.com/canonical/jimm/v3/pkg/api"
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
that have already been consumed with local users, you must specify a user mapping file
with the --user-mapping flag. This should point to a yaml file with a mapping of local
users to external users.
For example:

my-user-mapping.yaml:
'''
alice: alice@canonical.com
bob: bob@canonical.com
'''

The mapping must contain entries for all users that have access to the model and any offers
hosted within that model.
You can use the "juju show-model <model-name>" command to see the users that have access to
the model.
You can also use the "juju list-offers" command alongside "juju show-offer <offer-name>"
to see the users that have access to each offer.

Any users that you do not wish to be mapped must still be included with a null value or empty
string in place of the external user. This indicates that you are intentionally skipping this
local user, for example:
'''
alice: alice@canonical.com
bob: null # or ""
'''

The user mapping is consulted when relations are periodically validated. I.e. if an offer
was consumed by user "alice", when JIMM validates the relation it will understand that user
"alice" has been mapped and checks that "alice@canonical" still has access to the offer.
Revoking access from "alice@canonical.com" will result in the relation encountering an error.

It may not be possible to know all users that have have consumed offers from a model, but using
"juju show-offer <offer-name> --format yaml" you can see all users that have access to the
offer. This list should help determine which users to map in the user mapping file.

Any tools/scripts that refer to models by their full name (owner/name) will need to be
updated after migration to use the new external username or refer to models by their UUID.
`
	migrateModelCommandExample = `
    juju migrate alice/my-model my-jaas --backing-controller=controller-1 --user-mapping=./user-mapping.yaml
`
)

// MigrateAPI is an interface for the Juju API methods used by the migrate command.
type MigrateAPI interface {
	Close() error
	InitiateMigration(spec controllerapi.MigrationSpec) (string, error)
	ModelInfo(tags []names.ModelTag) ([]jujuparams.ModelInfoResult, error)
	ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
}

// NewMigrateModelCommand returns a command to migrate models.
func NewMigrateModelCommand() cmd.Command {
	cmd := &migrateModelCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())
	cmd.jimmAPIFunc = cmd.newJIMMClient
	cmd.jujuApiFunc = cmd.newJujuClient
	cmd.everyoneUser = "everyone@external"

	return modelcmd.WrapBase(cmd)
}

// migrateModelCommand migrates a model to JAAS from
// a controller that isn't registered with JAAS.
type migrateModelCommand struct {
	modelcmd.ControllerCommandBase

	out          cmd.Output
	jimmAPIFunc  func() (JIMMAPI, error)
	jujuApiFunc  func() (MigrateAPI, error)
	everyoneUser string

	dialOpts          *jujuapi.DialOpts
	targetController  string
	modelName         string
	backingController string
	userMappingFile   string
}

// Info implements Command.Info.
func (c *migrateModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "migrate",
		Args:     "<model-name> <jaas-name>",
		Purpose:  "Migrate models to JAAS, targetting the desired managed controller.",
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
		return errors.New("missing controller name and model target arguments")
	}
	// Note that modelName is a fully qualified model name, i.e. "owner/model-name".
	c.modelName = args[0]
	c.targetController = args[1]
	if c.userMappingFile == "" {
		return errors.New("missing user mapping file - please provide a user mapping file with the --user-mapping flag")
	}
	return nil
}

// Run implements Command.Run.
func (c *migrateModelCommand) Run(ctxt *cmd.Context) error {
	// Validate that the current controller exists.
	// This is the controller where the model currently resides.
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return fmt.Errorf("could not determine current controller: %w", err)
	}

	// Get the model info from the current controller.
	modelInfo, err := c.ClientStore().ModelByName(currentController, c.modelName)
	if err != nil {
		return fmt.Errorf("could not find model %q on controller %q: %v", c.modelName, currentController, err)
	}

	userMapping, err := c.parseUserMappingFile()
	if err != nil {
		return err
	}

	jujuAPI, err := c.jujuApiFunc()
	if err != nil {
		return fmt.Errorf("could not create Juju API client: %v", err)
	}
	defer jujuAPI.Close()

	err = c.validateUserMapping(userMapping, modelInfo.ModelUUID, c.modelName, jujuAPI)
	if err != nil {
		// validateUserMapping returns an error with suitable formatting for display.
		return err
	}

	token, err := c.prepareMigration(userMapping, modelInfo.ModelUUID)
	if err != nil {
		return fmt.Errorf("failure preparing migration: %v", err)
	}

	spec, err := c.getMigrationSpec(token, modelInfo.ModelUUID)
	if err != nil {
		return fmt.Errorf("could not get migration spec: %v", err)
	}

	events, err := jujuAPI.InitiateMigration(spec)
	if err != nil {
		return fmt.Errorf("could not initiate migration from controller %q: %v", currentController, err)
	}

	err = c.out.Write(ctxt, events)
	if err != nil {
		return fmt.Errorf("could not write migration events: %v", err)
	}
	return nil
}

// prepareMigration contacts the target controller (JIMM) to prepare
// it for migration and receive a migration token.
func (c *migrateModelCommand) prepareMigration(userMapping map[string]string, modelUUID string) (string, error) {
	jimmClient, err := c.jimmAPIFunc()
	if err != nil {
		return "", fmt.Errorf("could not create JIMM client: %v", err)
	}
	defer jimmClient.Close()
	response, err := jimmClient.PrepareModelMigration(&apiparams.PrepareModelMigrationRequest{
		BackingControllerName: c.backingController,
		UserMapping:           userMapping,
		ModelTag:              names.NewModelTag(modelUUID).String(),
	})
	if err != nil {
		return "", err
	}
	return response.Token, nil
}

func (c *migrateModelCommand) parseUserMappingFile() (map[string]string, error) {
	content, err := os.ReadFile(c.userMappingFile)
	if err != nil {
		return nil, fmt.Errorf("could not read user mapping file: %v", err)
	}
	userMapping := make(map[string]string)
	err = yaml.Unmarshal(content, &userMapping)
	if err != nil {
		return nil, fmt.Errorf("could not parse user mapping file: %v", err)
	}
	if len(userMapping) < 1 {
		return nil, fmt.Errorf("user mapping file is empty or not properly formatted")
	}
	return userMapping, nil
}

// getMigrationSpec creates the migration spec that will be used to initiate the migration.
func (c *migrateModelCommand) getMigrationSpec(token string, modelUUID string) (controllerapi.MigrationSpec, error) {
	store := c.ClientStore()
	accountDetails, err := store.AccountDetails(c.targetController)
	if err != nil {
		return controllerapi.MigrationSpec{}, fmt.Errorf("could not get account details for controller %q: %w", c.targetController, err)
	}

	controllerInfo, err := store.ControllerByName(c.targetController)
	if err != nil {
		return controllerapi.MigrationSpec{}, fmt.Errorf("could not find controller %q: %w", c.targetController, err)
	}

	return controllerapi.MigrationSpec{
		SkipUserChecks:        true,
		TargetControllerUUID:  controllerInfo.ControllerUUID,
		TargetControllerAlias: c.targetController,
		TargetAddrs:           controllerInfo.APIEndpoints,
		TargetCACert:          controllerInfo.CACert,
		ModelUUID:             modelUUID,
		TargetToken:           token,
		// The target user is not needed here, as the user details will be determined
		// by the contents of the migration token - a JWT token parsed by JIMM.
		// But Juju requires this field to be set, so we provide the user running the migration.
		TargetUser: accountDetails.User,
	}, nil
}

// validateUserMapping checks that the user mapping contains all necessary users
// that have access to the model and its offers.
func (c *migrateModelCommand) validateUserMapping(userMapping map[string]string, modelUUID, modelName string, jujuAPI MigrateAPI) error {
	modelInfo, err := jujuAPI.ModelInfo([]names.ModelTag{names.NewModelTag(modelUUID)})
	if err != nil {
		return fmt.Errorf("could not get model info: %v", err)
	}
	if len(modelInfo) == 0 {
		return fmt.Errorf("model %q not found", modelName)
	}
	modelUsers := modelInfo[0].Result.Users

	var missingUserMessages []string

	for _, user := range modelUsers {
		// Skip checks for the "everyone user" since the
		// supplied user mapping only needs specific users.
		if user.UserName == ofganames.EveryoneUser {
			continue
		}
		if _, ok := userMapping[user.UserName]; !ok {
			missingUserMessages = append(missingUserMessages, fmt.Sprintf("expected user %q who has %s access to the model", user.UserName, user.Access))
		}
	}

	// To list the model offers, we need the model name and owner as unfortunately
	// the model UUID is not sufficient to query the offers.
	unqualifiedModelName, ownerTag, err := jujuclient.SplitModelName(modelName)
	if err != nil {
		return err
	}
	filter := crossmodel.ApplicationOfferFilter{
		OwnerName: ownerTag.Id(),
		ModelName: unqualifiedModelName,
	}
	offers, err := jujuAPI.ListOffers(filter)
	if err != nil {
		return fmt.Errorf("could not list application offers: %v", err)
	}
	for _, offer := range offers {
		for _, user := range offer.Users {
			if user.UserName == ofganames.EveryoneUser {
				continue
			}
			if _, ok := userMapping[user.UserName]; !ok {
				missingUserMessages = append(missingUserMessages, fmt.Sprintf("expected user %q who has %s access to offer %q", user.UserName, user.Access, offer.OfferName))
			}
		}
	}
	if len(missingUserMessages) > 0 {
		return fmt.Errorf("user mapping is missing the following users:\n%s", strings.Join(missingUserMessages, "\n"))
	}

	return nil
}

func (c *migrateModelCommand) newJujuClient() (MigrateAPI, error) {
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %v", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return jujuMigrateAPI{apiCaller: apiCaller}, nil
}

// jujuMigrateAPI is an implementation of the MigrateAPI interface
// that uses multiple Juju API clients to perform the necessary operations.
type jujuMigrateAPI struct {
	apiCaller jujuapi.Connection
}

// Close implements the MigrateAPI interface.
func (j jujuMigrateAPI) Close() error {
	return j.apiCaller.Close()
}

// InitiateMigration implements the MigrateAPI interface.
func (j jujuMigrateAPI) InitiateMigration(spec controllerapi.MigrationSpec) (string, error) {
	client := controllerapi.NewClient(j.apiCaller)
	return client.InitiateMigration(spec, false)
}

// ModelInfo implements the MigrateAPI interface.
func (j jujuMigrateAPI) ModelInfo(tags []names.ModelTag) ([]jujuparams.ModelInfoResult, error) {
	client := modelmanager.NewClient(j.apiCaller)
	return client.ModelInfo(tags)
}

// ListOffers implements the MigrateAPI interface.
func (j jujuMigrateAPI) ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	client := applicationoffers.NewClient(j.apiCaller)
	return client.ListOffers(filters...)
}

// newJIMMClient creates a new JIMM client for the migration command.
// It assumes the target controller is a JIMM controller.
func (c *migrateModelCommand) newJIMMClient() (JIMMAPI, error) {
	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), c.targetController, "", nil)
	if err != nil {
		return nil, err
	}

	return jimmAPI.NewClient(apiCaller), nil
}
