// Copyright 2021 Canonical Ltd.

package cmd

import (
	"fmt"
	"io/ioutil"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	jujucmdcommon "github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	jimmjujuapi "github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

var (
	addCloudToControllerCommandDoc = `
	add-cloud-to-controller command adds the specified cloud to a specific 
	controller on jimm.

	One can specify a cloud definition via a yaml file passed with the --cloud 
	flag. If the flag is missing, the command will assume the cloud definition
	is already known and will error otherwise.

	Example:
		jimmctl add-cloud-to-controller <controller_name> <cloud_name>
		jimmctl add-cloud-to-controller <controller_name> <cloud_name> --cloud=<cloud_file_path> 
`
)

// NewAddControllerCommand returns a command to add a cloud to a specific
// controller in JIMM.
func NewAddCloudToControllerCommand() cmd.Command {
	cmd := &addCloudToControllerCommand{
		store:           jujuclient.NewFileClientStore(),
		cloudByNameFunc: jujucmdcommon.CloudByName,
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

	cloudByNameFunc func(string) (*cloud.Cloud, error)
	store           jujuclient.ClientStore
	dialOpts        *jujuapi.DialOpts
}

// Info implements Command.Info.
func (c *addCloudToControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-cloud-to-controller",
		Purpose: "Add cloud to specific controller in jimm",
		Doc:     addCloudToControllerCommandDoc,
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
	f.StringVar(&c.cloudDefinitionFile, "cloud", "", "The path to the cloud's definition file.")
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
		newCloud, err = c.readCloudFromFile(ctxt)
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

	err = c.addCloudToController(ctxt, newCloud)
	if err != nil {
		return errors.E(err, fmt.Sprintf("error adding cloud to controller: %v", err))
	}

	return nil
}

func (c *addCloudToControllerCommand) addCloudToController(ctxt *cmd.Context, cloud *cloud.Cloud) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine the current controller")
	}
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}
	client := api.NewClient(apiCaller)

	params := &apiparams.AddCloudToControllerRequest{
		ControllerName: c.dstControllerName,
		AddCloudArgs: params.AddCloudArgs{
			Name:  c.cloudName,
			Cloud: jimmjujuapi.CloudToParams(*cloud),
			Force: &c.force,
		},
	}

	err = client.AddCloudToController(params)
	if err != nil {
		return errors.E(err)
	}

	ctxt.Infof("Cloud %q added to controller %q.", c.cloudName, c.dstControllerName)
	return nil
}

func (c *addCloudToControllerCommand) readCloudFromFile(ctxt *cmd.Context) (*cloud.Cloud, error) {
	r := &jujucmdcloud.CloudFileReader{
		CloudMetadataStore: &cloudToCommandAdapter{},
		CloudName:          c.cloudName,
	}
	newCloud, err := r.ReadCloudFromFile(c.cloudDefinitionFile, ctxt)
	if err != nil {
		return nil, errors.E(err)
	}
	return newCloud, nil
}

type cloudToCommandAdapter struct {
	jujucmdcloud.CloudMetadataStore
}

// ReadCloudData implements CloudMetadataStore.ReadCloudData.
func (cloudToCommandAdapter) ReadCloudData(path string) ([]byte, error) {
	return ioutil.ReadFile(path)
}

// ParseOneCloud implements CloudMetadataStore.ParseOneCloud.
func (cloudToCommandAdapter) ParseOneCloud(data []byte) (cloud.Cloud, error) {
	return cloud.ParseOneCloud(data)
}

// PublicCloudMetadata implements CloudMetadataStore.PublicCloudMetadata.
func (cloudToCommandAdapter) PublicCloudMetadata(searchPaths ...string) (map[string]cloud.Cloud, bool, error) {
	return cloud.PublicCloudMetadata(searchPaths...)
}

// PersonalCloudMetadata implements CloudMetadataStore.PersonalCloudMetadata.
func (cloudToCommandAdapter) PersonalCloudMetadata() (map[string]cloud.Cloud, error) {
	return cloud.PersonalCloudMetadata()
}

// WritePersonalCloudMetadata implements CloudMetadataStore.WritePersonalCloudMetadata.
func (cloudToCommandAdapter) WritePersonalCloudMetadata(cloudsMap map[string]cloud.Cloud) error {
	return cloud.WritePersonalCloudMetadata(cloudsMap)
}
