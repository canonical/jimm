// Copyright 2025 Canonical.

package cmd

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	jimmAPI "github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	bootstrapDoc = `
Requests the JIMM server to bootstrap a Juju controller.
The controller will be created asychronously on the specificed
cloud and region. 

By default the command will wait for the bootstrap job to complete
while printing the job logs. Note that the logs will not follow the 
--output flag and will always be printed to stdout. This can allow
you to send the initial output with the job ID to a file, while the
logs are streamed to stdout.

Use the --detach flag to start the bootstrap job and return immediately,
printing only the job ID, without waiting for the job to complete.

The final argument, version, denotes the Juju controller to be bootstrapped.
`
	bootstrapExamples = `
	juju [jaas] bootstrap <cloud[/region]> <controller name> <controller version>
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8
`
)

// bootstrapCommand starts a bootstrap jobon the controller.
type bootstrapCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store            jujuclient.ClientStore
	bootstrapAPIFunc func() (JIMMAPI, error)

	cloud             string
	region            string
	controllerName    string
	controllerVersion string
	timeout           int
	detach            bool
}

// NewBootstrapStartCommand returns a command to start a job
// that will bootstrap a Juju controller.
func NewBootstrapStartCommand() cmd.Command {
	cmd := &bootstrapCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.bootstrapAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// Init implements modelcmd.Command.
func (c *bootstrapCommand) Init(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("expected at least 3 arguments, got %d", len(args))
	}
	c.cloud = args[0]
	if i := strings.IndexRune(c.cloud, '/'); i > 0 {
		c.cloud, c.region = c.cloud[:i], c.cloud[i+1:]
	}
	if ok := names.IsValidCloud(c.cloud); !ok {
		return fmt.Errorf("cloud name %q not valid", c.cloud)
	}
	c.controllerName = args[1]
	c.controllerVersion = args[2]

	return nil
}

// SetFlags implements modelcmd.Command.
func (c *bootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "json", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.IntVar(&c.timeout, "timeout", 0, "The timeout in seconds for the bootstrap operation.")
	f.BoolVar(&c.detach, "detach", false, "If set, the command will start the bootstrap job and return immediately with the job ID, without waiting for the job to complete.")
	// TODO(ale8k): Support passing cloud & cloudcredential files, for now we're looking up clouds and credentials added to the store.
	// See cmd/juju/cloud/add.go L311 on a nice way to do this and credential will be somewhere in there too.
}

// Info implements modelcmd.Command.
func (c *bootstrapCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "bootstrap",
		Args:     "<cloud name>[/region] <controller name> <juju version>",
		Purpose:  "Bootstrap a Juju controller via JIMM",
		Doc:      bootstrapDoc,
		Examples: bootstrapExamples,
	})
}

// Run implements modelcmd.Command.
func (c *bootstrapCommand) Run(ctxt *cmd.Context) error {
	// We use [jujucloud.CloudByName] and not [common.CloudByName] as the JIMM bootstrap
	// will NOT support builtin clouds (localhost, microk8s, docker-desktop, etc.).
	bootstrapCloud, err := jujucloud.CloudByName(c.cloud)
	if err != nil {
		return fmt.Errorf("failed to get cloud %q: %w", c.cloud, err)
	}

	bootstrapCredential, err := c.store.CredentialForCloud(c.cloud)
	if err != nil {
		return fmt.Errorf("failed to get credential for cloud %q: %w", c.cloud, err)
	}

	req := apiparams.BootstrapStartParams{
		CloudName:         c.cloud,
		RegionName:        c.region,
		ControllerName:    c.controllerName,
		ControllerVersion: c.controllerVersion,
		Cloud:             cloudToParams(*bootstrapCloud),
		Credential:        *bootstrapCredential,

		Flags: apiparams.BootstrapFlags{
			Timeout: c.timeout,
		},
	}

	client, err := c.bootstrapAPIFunc()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %v", err)
	}
	defer client.Close()

	resp, err := client.Bootstrap(&req)
	if err != nil {
		return err
	}

	if c.detach {
		fmt.Printf(`
Bootstrap job started.
You can track the progress via bootstrap-status with the job ID:
	juju [jaas] bootstrap-status %s

	`,
			resp.JobID,
		)
	} else {
		fmt.Printf(`
Starting bootstrap job.

Should you cancel this process, you can track the progress via bootstrap-status with the job ID:
	juju [jaas] bootstrap-status %s

	`,
			resp.JobID,
		)
	}

	if c.detach {
		return nil
	}

	// Don't use c.out for the logs since c.out
	// attempts to format the output.

	poller := logPoller{
		client:              client,
		jobId:               resp.JobID,
		sleepBetweenGetLogs: sleepBetweenGetLogs,
		out:                 ctxt.Stdout,
		follow:              true,
	}

	return poller.watchBootstrapLogs()
}

func (c *bootstrapCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %v", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return jimmAPI.NewClient(apiCaller), nil
}

// CloudToParams converts a jujucloud.Cloud to a jujuparams.Cloud.
// Copied from api/client/cloud/cloud.go.
func cloudToParams(cloud jujucloud.Cloud) jujuparams.Cloud {
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	regions := make([]jujuparams.CloudRegion, len(cloud.Regions))
	for i, region := range cloud.Regions {
		regions[i] = jujuparams.CloudRegion{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	var regionConfig map[string]map[string]interface{}
	for r, attr := range cloud.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]map[string]interface{})
		}
		regionConfig[r] = attr
	}
	return jujuparams.Cloud{
		Type:              cloud.Type,
		HostCloudRegion:   cloud.HostCloudRegion,
		AuthTypes:         authTypes,
		Endpoint:          cloud.Endpoint,
		IdentityEndpoint:  cloud.IdentityEndpoint,
		StorageEndpoint:   cloud.StorageEndpoint,
		Regions:           regions,
		CACertificates:    cloud.CACertificates,
		SkipTLSVerify:     cloud.SkipTLSVerify,
		Config:            cloud.Config,
		RegionConfig:      regionConfig,
		IsControllerCloud: cloud.IsControllerCloud,
	}
}
