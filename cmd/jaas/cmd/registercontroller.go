// Copyright 2025 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"sigs.k8s.io/yaml"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

var (
	// stdinMarkers contains file names that are taken to be stdin.
	stdinMarkers = []string{"-"}

	registerControllerCommandDoc = `
Registers a controller with JIMM.

Using the controller name provided, this command will inspect your
Juju client store for details on the specified controller.

Note that by default, this command assumes the controller has the public-hostname
field set, which will define the preferred address JIMM will use to contact the
controller. Use of a public address will also ignore any custom CA cert in your
local client store and assumes the server is secured with a public certificate.

Use the --local flag if the server is not configured with a public address or to
ignore the controller's public-hostname and use the custom CA of the controller.

A yaml formatted file can also be used as input for cases where the controller
is not available on the client. Using the --file will validate that the provided
controller name matches the name in the yaml file.
Using --file will ignore other flags like --public-address and --local.

Use the --dry-run flag to generate a sample file without registering the controller.
This can be used later as input to register-controller.
`
	registerControllerCommandExample = `
    juju register-controller mycontroller
    juju register-controller mycontroller --local
`
)

// NewRegisterControllerCommand returns a command to register a controller.
func NewRegisterControllerCommand() cmd.Command {
	cmd := &registerControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// registerControllerCommand register a controller.
type registerControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store          jujuclient.ClientStore
	dialOpts       *jujuapi.DialOpts
	file           cmd.FileVar
	local          bool
	tlsHostname    string
	controllerName string
	publicAddress  string
	dryRun         bool
}

func (c *registerControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "register-controller",
		Purpose:  "Add controller to jimm",
		Args:     "<filepath>",
		Doc:      registerControllerCommandDoc,
		Examples: registerControllerCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *registerControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	c.file.StdinMarkers = stdinMarkers
	f.BoolVar(&c.local, "local", false, "If local flag is specified, then the local API addresses and CA cert of the controller will be used.")
	f.BoolVar(&c.dryRun, "dry-run", false, "Dry-run enabled will only print the controller details.")
	f.StringVar(&c.tlsHostname, "tls-hostname", "", "Specify the hostname for TLS verification.")
	f.StringVar(&c.file.Path, "file", "", "Specify a file-path for controller details, use '-' to read from stdin.")
	f.StringVar(&c.publicAddress, "public-address", "", "Specify a custom public address to use for dialing the controller.")
}

// Init implements the cmd.Command interface.
func (c *registerControllerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("controller name not specified")
	}
	c.controllerName = args[0]
	if len(args) > 1 {
		return errors.New("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *registerControllerCommand) Run(ctxt *cmd.Context) error {
	data, err := c.getControllerDetails(ctxt)
	if err != nil {
		return err
	}
	var params apiparams.AddControllerRequest
	if err = unmarshalControllerDetails(&params, data); err != nil {
		return err
	}
	if c.controllerName != params.Name {
		return errors.New(fmt.Sprintf("provided controller name doesn't match, %s != %s", c.controllerName, params.Name))
	}
	if c.dryRun {
		return c.out.Write(ctxt, params)
	}

	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.Annotate(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	info, err := client.AddController(&params)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, info)
	if err != nil {
		return err
	}
	return nil
}

func unmarshalControllerDetails(v interface{}, data []byte) error {
	err := yaml.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	return nil
}

func (c *registerControllerCommand) getControllerDetails(ctxt *cmd.Context) ([]byte, error) {
	if c.file.Path != "" {
		return c.file.Read(ctxt)
	}

	controller, err := c.store.ControllerByName(c.controllerName)
	if err != nil {
		return nil, errors.Mask(err)
	}

	accountDetails, err := c.store.AccountDetails(c.controllerName)
	if err != nil {
		return nil, errors.Mask(err)
	}

	info := apiparams.AddControllerRequest{
		UUID:          controller.ControllerUUID,
		Name:          c.controllerName,
		APIAddresses:  controller.APIEndpoints,
		Username:      accountDetails.User,
		Password:      accountDetails.Password,
		PublicAddress: controller.PublicDNSName,
		TLSHostname:   c.tlsHostname,
	}

	if c.local {
		info.PublicAddress = ""
		info.CACertificate = controller.CACert
	}

	if c.publicAddress != "" {
		info.PublicAddress = c.publicAddress
	}

	data, err := yaml.Marshal(info)
	if err != nil {
		return nil, errors.Mask(err)
	}
	return data, nil
}
