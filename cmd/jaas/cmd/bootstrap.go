// Copyright 2026 Canonical.

package cmd

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

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

Config options for the bootstrap process can be specified via one or more
--config options. Each --config option can either be a path to a YAML file
containing config options, or a key=value pair. If multiple --config options
are specified, they will be merged together, with later options taking
precedence over earlier ones. Key=value pairs will take precedence over
file contents.

These config options must match the config options supported by the Juju CLI
for the version of Juju being bootstrapped. See the Juju documentation for
the version specified for the full list of supported bootstrap config
options.

Note that some config options may not be specified as they will automatically
be set.
These are:

- login-token-refresh-url

Bootstrapping to a k8s cluster requires that the service set up to handle
requests to the controller be accessible outside the cluster. Typically this
means a service type of LoadBalancer is needed, and Juju does create such a
service if it knows it is supported by the cluster. This is performed by
interrogating the cluster for a well known managed deployment such as microk8s,
GKE or EKS.

See the Juju bootstrap documentation for more details and how to configure
bootstrap for a Kubernetes cluster Juju does not recognise.

Note that JIMM will internally do the following:
- download the juju CLI matching the desired controller version
- bootstrap a new controller
- register the controller with JIMM
`
	bootstrapExamples = `
	juju [jaas] bootstrap <cloud[/region]> <controller name> <controller version>
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --config controller-service-type=loadbalancer
`
)

// bootstrapCommand starts a bootstrap jobon the controller.
type bootstrapCommand struct {
	jaasCommandBase
	out cmd.Output

	cloud             string
	region            string
	controllerName    string
	controllerVersion string

	// Flags

	credentialName string
	detach         bool
	config         common.ConfigFlag
}

// NewBootstrapStartCommand returns a command to start a job
// that will bootstrap a Juju controller.
func NewBootstrapStartCommand() cmd.Command {
	cmd := &bootstrapCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

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
	f.StringVar(&c.credentialName, "credential", "", "The name of the cloud credential to use for bootstrapping. Only required if more than one credential is available for the cloud.")
	f.Var(&c.config, "config",
		"Specify a configuration file, or one or more configuration options.\n    (`--config config.yaml [--config key=value ...])`")
	f.BoolVar(&c.detach, "detach", false, "If set, the command will start the bootstrap job and return immediately with the job ID, without waiting for the job to complete.")
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

	// If the cloud is a public cloud (AWS, Azure, etc), we clear bootstrapCloud to avoid sending
	// unnecessary info, letting the server decide the cloud endpoints, etc.
	// Regardless, the server uses its own cloud definition if it identifies the cloud is a public cloud.
	publicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return fmt.Errorf("failed to get public cloud metadata: %w", err)
	}
	for name := range publicClouds {
		if name == c.cloud {
			bootstrapCloud = nil
			break
		}
	}

	cloudCreds, err := c.ClientStore().CredentialForCloud(c.cloud)
	if err != nil {
		return fmt.Errorf("failed to get credential for cloud %q: %w", c.cloud, err)
	}

	var bootstrapCred jujucloud.Credential
	switch {
	case len(cloudCreds.AuthCredentials) == 1 && c.credentialName == "":
		// If there's only one credential and the user didn't specify a credential name, use it.
		for _, cred := range cloudCreds.AuthCredentials {
			bootstrapCred = cred
			break
		}
	case c.credentialName != "":
		// If a credential name is provided, use it.
		var ok bool
		bootstrapCred, ok = cloudCreds.AuthCredentials[c.credentialName]
		if !ok {
			return fmt.Errorf("no credential found with name %q", c.credentialName)
		}
	case cloudCreds.DefaultCredential != "" && c.credentialName == "":
		// If there's a default credential and the user didn't specify a credential name, use it.
		var ok bool
		bootstrapCred, ok = cloudCreds.AuthCredentials[cloudCreds.DefaultCredential]
		if !ok {
			return fmt.Errorf("default credential %q not found for cloud %q", cloudCreds.DefaultCredential, c.cloud)
		}
	default:
		// If there are multiple credentials and no name is provided, return an error.
		return fmt.Errorf("multiple credentials found for cloud %q, please set a default or specify one using --credential", c.cloud)
	}

	configValues, err := c.config.ReadAttrs(ctxt)
	if err != nil {
		return fmt.Errorf("failed to read config values: %v", err)
	}

	stringConfigValues := make(map[string]string, len(configValues))
	for k, v := range configValues {
		strVal, ok := v.(string)
		if !ok {
			return fmt.Errorf("config value for %q must be a string, got %T", k, v)
		}
		stringConfigValues[k] = strVal
	}

	req := apiparams.BootstrapParams{
		ControllerName:    c.controllerName,
		ControllerVersion: c.controllerVersion,
		Cloud:             cloudToParams(c.cloud, c.region, bootstrapCloud),
		Credential: jujuparams.CloudCredential{
			Attributes: bootstrapCred.Attributes(),
			AuthType:   string(bootstrapCred.AuthType()),
		},
		BootstrapOptions: apiparams.BootstrapOptions{
			BootstrapConfig: stringConfigValues,
		},
	}

	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %v", err)
	}
	defer client.Close()

	resp, err := client.StartBootstrap(&req)
	if err != nil {
		return err
	}

	if c.detach {
		fmt.Printf(`
Bootstrap started.
You can track the progress via bootstrap-status with the job ID:
	juju [jaas] bootstrap-status %s

	`,
			resp.JobID,
		)
	} else {
		fmt.Printf(`
Starting bootstrap.

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

	poller := bootstrapLogPoller{
		client:              client,
		jobId:               resp.JobID,
		sleepBetweenGetLogs: sleepBetweenGetLogs,
		out:                 ctxt.Stdout,
		follow:              true,
	}

	return poller.watchBootstrapLogs()
}

// cloudToParams converts a jujucloud.Cloud to the bootstrap request cloud shape.
func cloudToParams(cloudName, regionName string, cloud *jujucloud.Cloud) apiparams.BootstrapCloud {
	paramsCloud := apiparams.BootstrapCloud{
		Name: cloudName,
		Region: apiparams.BootstrapCloudRegion{
			Name: regionName,
		},
	}
	if cloud == nil {
		return paramsCloud
	}
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	paramsCloud.Type = cloud.Type
	paramsCloud.AuthTypes = authTypes
	paramsCloud.Endpoint = cloud.Endpoint
	paramsCloud.CACertificates = cloud.CACertificates
	paramsCloud.HostCloudRegion = cloud.HostCloudRegion
	paramsCloud.Config = cloud.Config
	for _, region := range cloud.Regions {
		if region.Name == regionName {
			paramsCloud.Region = apiparams.BootstrapCloudRegion{
				Name:             region.Name,
				Endpoint:         region.Endpoint,
				IdentityEndpoint: region.IdentityEndpoint,
				StorageEndpoint:  region.StorageEndpoint,
			}
			break
		}
	}
	return paramsCloud
}
