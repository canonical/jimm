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

Bootstrapping to a k8s cluster requires that the service set up to handle
requests to the controller be accessible outside the cluster. Typically this
means a service type of LoadBalancer is needed, and Juju does create such a
service if it knows it is supported by the cluster. This is performed by
interrogating the cluster for a well known managed deployment such as microk8s,
GKE or EKS.

When bootstrapping to a Kubernetes cluster Juju does not recognise, there's no
guarantee a load balancer is available, so Juju defaults to a controller
service type of ClusterIP. In the case of bootstrapping via JIMM, this will 
not work unless JIMM is deployed within the same cluster. There are three bootstrap
options available to tell Juju how to set up the controller service. Part of
the solution may require a load balancer for the cluster to be set up manually
first, or perhaps an external Kubernetes service via a FQDN will be used
(this is a cluster specific implementation decision which Juju needs to be
informed about so it can set things up correctly). The three relevant bootstrap
options are (see list of bootstrap config items below for a full explanation):

- controller-service-type
- controller-external-name 
- controller-external-ips

Juju advertises those addresses to other controllers (including JIMM), so they must be resolveable from
other controllers for cross-model (cross-controller, actually) relations to work.
`
	bootstrapExamples = `
	juju [jaas] bootstrap <cloud[/region]> <controller name> <controller version>
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --controller-service-type=loadbalancer
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

	// Flags

	credentialName         string
	timeout                int
	publicDNSAddress       string
	controllerServiceType  string
	controllerExternalIPs  string
	controllerExternalName string
	detach                 bool
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
	f.StringVar(&c.credentialName, "credential", "", "The name of the cloud credential to use for bootstrapping. Only required if more than one credential is available for the cloud.")
	f.IntVar(&c.timeout, "timeout", 0, "The timeout in seconds for the bootstrap operation.")
	f.StringVar(&c.publicDNSAddress, "public-dns-address", "", "Public DNS address (with port) of the controller")
	f.StringVar(
		&c.controllerServiceType,
		"controller-service-type",
		"",
		"Controls the kubernetes service type for Juju controllers, see https://kubernetes.io/docs/reference/kubernetes-api/service-resources/service-v1/#ServiceSpec valid values are one of cluster, loadbalancer, external",
	)
	f.StringVar(&c.controllerExternalIPs, "controller-external-ips", "", "Specifies a comma separated list of external IPs for a k8s controller of type external")
	f.StringVar(&c.controllerExternalName, "controller-external-name", "", "Sets the external name for a k8s controller of type external")
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

	cloudCreds, err := c.store.CredentialForCloud(c.cloud)
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

	req := apiparams.BootstrapStartParams{
		CloudName:         c.cloud,
		RegionName:        c.region,
		ControllerName:    c.controllerName,
		ControllerVersion: c.controllerVersion,
		Cloud:             cloudToParams(*bootstrapCloud),
		Credential: jujuparams.CloudCredential{
			Attributes: bootstrapCred.Attributes(),
			AuthType:   string(bootstrapCred.AuthType()),
		},

		Flags: apiparams.BootstrapFlags{
			Timeout:                c.timeout,
			PublicDNSAddress:       c.publicDNSAddress,
			ControllerServiceType:  c.controllerServiceType,
			ControllerExternalIPs:  c.controllerExternalIPs,
			ControllerExternalName: c.controllerExternalName,
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
