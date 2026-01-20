// Copyright 2025 Canonical.

package cmd

import (
	"fmt"
	"os"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	jujucmdcommon "github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	jimmjujuapi "github.com/canonical/jimm/v3/internal/jujuapi"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	addCloudToControllerCommandDoc = `
Adds the specified cloud to a specific controller on JIMM.

One can specify a cloud definition via a yaml file passed with the --cloud 
flag. If the flag is missing, the command will assume the cloud definition
is already known and will error otherwise.
`
	addCloudToControllerExample = `
    juju add-cloud mycontroller mycloud
    juju add-cloud mycontroller mycloud --cloud=./cloud-definition.yaml
`
)

// NewAddControllerCommand returns a command to add a cloud to a specific
// controller in JIMM.
func NewAddCloudToControllerCommand() cmd.Command {
	cmd := &addCloudToControllerCommand{
		cloudByNameFunc: jujucmdcommon.CloudByName,
		jimmAPIFunc:     NewClient,
	}

	return modelcmd.WrapBase(cmd)
}

// addControllerCommand adds a controller.
type addCloudToControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	// cloudName is the name of the cloud to add.
	cloudName string

	// cloudDefinitionFile is the name of the cloud file.
	cloudDefinitionFile string

	// dstControllerName is the name of the controller in JIMM where the cloud
	// should be added to.
	dstControllerName string

	// force skips checks that verify whether the cloud that is being added is
	// compatible with the cloud on which the controller is running.
	force bool

	jimmAPIFunc     APIClientFunc
	cloudByNameFunc func(string) (*cloud.Cloud, error)
}

// Info implements Command.Info.
func (c *addCloudToControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-cloud",
		Args:     "<controller_name> <cloud_name>",
		Purpose:  "Add cloud to specific controller in jimm",
		Doc:      addCloudToControllerCommandDoc,
		Examples: addCloudToControllerExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *addCloudToControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})

	f.BoolVar(&c.force, "force", false, "Forces the cloud to be added to the controller")
	f.StringVar(&c.cloudDefinitionFile, "cloud", "", "The path to the cloud's definition file. The cloud name must be present in the file.")
}

// Init implements the cmd.Command interface.
func (c *addCloudToControllerCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.E("missing arguments")
	}
	if len(args) > 2 {
		return errors.E("too many arguments")
	}
	c.dstControllerName = args[0]
	if ok := names.IsValidControllerName(c.dstControllerName); !ok {
		return errors.E("invalid controller name %q", c.dstControllerName)
	}
	c.cloudName = args[1]
	if ok := names.IsValidCloud(c.cloudName); !ok {
		return errors.E("invalid cloud name %q", c.cloudName)
	}

	return nil
}

// Run implements Command.Run.
func (c *addCloudToControllerCommand) Run(ctxt *cmd.Context) error {
	var newCloud *cloud.Cloud
	var err error
	if c.cloudDefinitionFile != "" {
		newCloud, err = c.readCloudFromFile()
		if err != nil {
			return errors.E(err, fmt.Sprintf("error reading cloud from file: %v", err))
		}
	} else {
		// It's possible that the user wants to add an existing cloud to a controller,
		// so let's see if we can find the cloud.
		newCloud, err = c.cloudByNameFunc(c.cloudName)
		if err != nil {
			return errors.E("could not find existing cloud, please provide a cloud file")
		}
	}

	// All clouds must have at least one default region.
	if len(newCloud.Regions) == 0 {
		newCloud.Regions = []cloud.Region{{Name: cloud.DefaultCloudRegion}}
	}

	jimmAPI, err := c.jimmAPIFunc(nil)
	if err != nil {
		return errors.E(err, "could not create JIMM API client")
	}
	defer jimmAPI.Close()

	if err := jimmAPI.AddCloudToController(&apiparams.AddCloudToControllerRequest{
		ControllerName: c.dstControllerName,
		AddCloudArgs: params.AddCloudArgs{
			Name:  c.cloudName,
			Cloud: jimmjujuapi.CloudToParams(*newCloud),
			Force: &c.force,
		},
	}); err != nil {
		return errors.E(err)
	}
	ctxt.Infof("Cloud %q added to controller %q.", c.cloudName, c.dstControllerName)

	return nil
}

func (c *addCloudToControllerCommand) readCloudFromFile() (*cloud.Cloud, error) {
	cloudDefinitionData, err := os.ReadFile(c.cloudDefinitionFile)
	if err != nil {
		return nil, errors.E(err)
	}
	specifiedClouds, err := cloud.ParseCloudMetadata(cloudDefinitionData)
	if err != nil {
		return nil, errors.E(err)
	}
	if len(specifiedClouds) == 0 {
		return nil, errors.E("no clouds found in parsed yaml, please validate yaml keys")
	}
	var ok bool
	foundCloud, ok := specifiedClouds[c.cloudName]
	if !ok {
		return nil, errors.E(fmt.Sprintf("cloud %q not found in file %q", c.cloudName, c.cloudDefinitionFile))
	}

	return &foundCloud, nil
}
