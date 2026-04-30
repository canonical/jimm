// Copyright 2026 Canonical.

package cmd

import (
	"fmt"
	"maps"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
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

Saved controller profiles can be applied with --profile. The profile's saved
cloud definition and bootstrap options are used as defaults, and any explicit
command line arguments or flags override those saved values.

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
options. Additional bootstrap settings can be supplied with --bootstrap-base,
--bootstrap-constraints, --constraints, --model-default, and --storage-pool,
these align with the corresponding Juju CLI options.

Note that some config options will be automatically set but can be overriden.
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
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --profile production-aws
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --config controller-service-type=loadbalancer
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --bootstrap-base ubuntu@24.04 --bootstrap-constraints mem=8G --constraints arch=amd64
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --storage-pool name=controller-pool --storage-pool type=ebs --config audit-log-enabled=true
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
	profileName    string
	bootstrapBase  string
	constraints    common.ConstraintsFlag
	bootstrapCons  common.BootstrapConstraintsFlag
	config         common.ConfigFlag
	modelDefaults  common.ConfigFlag
	storagePool    common.ConfigFlag
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
	if c.bootstrapBase != "" {
		if _, err := corebase.ParseBaseFromString(c.bootstrapBase); err != nil {
			return fmt.Errorf("invalid bootstrap base %q: %w", c.bootstrapBase, err)
		}
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
	f.StringVar(&c.profileName, "profile", "", "Apply a saved controller profile before processing explicit bootstrap flags. Explicit command line values override the profile.")
	f.StringVar(&c.bootstrapBase, "bootstrap-base", "", "Specify the base of the bootstrap machine.")
	f.Var(&c.bootstrapCons, "bootstrap-constraints", "Specify bootstrap machine constraints.")
	f.Var(&c.constraints, "constraints", "Set model constraints")
	f.Var(&c.config, "config",
		"Specify a configuration file, or one or more configuration options.\n    (`--config config.yaml [--config key=value ...])`")
	f.Var(&c.modelDefaults, "model-default",
		"Specify a configuration file, or one or more configuration options to be set for all models, unless otherwise specified.\n    (`--model-default config.yaml [--model-default key=value ...])`")
	f.Var(&c.storagePool, "storage-pool",
		"Specify options for an initial storage pool. 'name' and 'type' are required, plus any additional attributes.\n    (`--storage-pool pool-config.yaml [--storage-pool key=value ...])`")
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

	bootstrapOptions, err := c.bootstrapOptions(ctxt)
	if err != nil {
		return err
	}

	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %v", err)
	}
	defer client.Close()

	bootstrapCloudParams := cloudToParams(c.cloud, c.region, bootstrapCloud)
	if c.profileName != "" {
		resp, err := client.GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: c.profileName})
		if err != nil {
			return fmt.Errorf("failed to get controller profile %q: %w", c.profileName, err)
		}
		bootstrapCloudParams = mergeBootstrapCloud(resp.Cloud, bootstrapCloudParams)
		bootstrapOptions = mergeBootstrapOptions(resp.BootstrapOptions, bootstrapOptions)
	}

	req := apiparams.BootstrapParams{
		ControllerName:    c.controllerName,
		ControllerVersion: c.controllerVersion,
		Cloud:             bootstrapCloudParams,
		Credential: jujuparams.CloudCredential{
			Attributes: bootstrapCred.Attributes(),
			AuthType:   string(bootstrapCred.AuthType()),
		},
		BootstrapOptions: bootstrapOptions,
	}

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

// mergeBootstrapCloud merges parameters from a profile with those explicitly provided.
// All fields are copied to avoid re-using the underlying data structures from the profile.
func mergeBootstrapCloud(profile, explicit apiparams.BootstrapCloud) apiparams.BootstrapCloud {
	merged := profile
	if explicit.Name != "" {
		merged.Name = explicit.Name
	}
	if explicit.Type != "" {
		merged.Type = explicit.Type
	}
	if len(explicit.AuthTypes) > 0 {
		merged.AuthTypes = append([]string(nil), explicit.AuthTypes...)
	} else if len(merged.AuthTypes) > 0 {
		// Create a new slice to avoid sharing the underlying array with the profile.
		merged.AuthTypes = append([]string(nil), merged.AuthTypes...)
	}
	if len(explicit.CACertificates) > 0 {
		merged.CACertificates = append([]string(nil), explicit.CACertificates...)
	} else if len(merged.CACertificates) > 0 {
		// Create a new slice to avoid sharing the underlying array with the profile.
		merged.CACertificates = append([]string(nil), merged.CACertificates...)
	}
	merged.Config = mergeMaps(profile.Config, explicit.Config)
	if explicit.Endpoint != "" {
		merged.Endpoint = explicit.Endpoint
	}
	if explicit.HostCloudRegion != "" {
		merged.HostCloudRegion = explicit.HostCloudRegion
	}
	merged.Region = mergeBootstrapCloudRegion(profile.Region, explicit.Region)
	return merged
}

// mergeBootstrapCloudRegion merges parameters from a profile with those explicitly provided for the cloud region.
func mergeBootstrapCloudRegion(profile, explicit apiparams.BootstrapCloudRegion) apiparams.BootstrapCloudRegion {
	merged := profile
	if explicit.Name != "" {
		merged.Name = explicit.Name
	}
	if explicit.Endpoint != "" {
		merged.Endpoint = explicit.Endpoint
	}
	if explicit.IdentityEndpoint != "" {
		merged.IdentityEndpoint = explicit.IdentityEndpoint
	}
	if explicit.StorageEndpoint != "" {
		merged.StorageEndpoint = explicit.StorageEndpoint
	}
	return merged
}

func (c *bootstrapCommand) bootstrapOptions(ctxt *cmd.Context) (apiparams.BootstrapOptions, error) {
	bootstrapConfig, err := readStringMapFlag(ctxt, &c.config, "config")
	if err != nil {
		return apiparams.BootstrapOptions{}, err
	}
	modelDefault, err := readStringMapFlag(ctxt, &c.modelDefaults, "model-default")
	if err != nil {
		return apiparams.BootstrapOptions{}, err
	}
	bootstrapConstraints, err := parseConstraintFlag(ctxt, []string(c.bootstrapCons), "bootstrap-constraints")
	if err != nil {
		return apiparams.BootstrapOptions{}, err
	}
	modelConstraints, err := parseConstraintFlag(ctxt, []string(c.constraints), "constraints")
	if err != nil {
		return apiparams.BootstrapOptions{}, err
	}
	storagePool, err := readStoragePoolFlag(ctxt, &c.storagePool)
	if err != nil {
		return apiparams.BootstrapOptions{}, err
	}

	return apiparams.BootstrapOptions{
		BootstrapBase:        c.bootstrapBase,
		BootstrapConstraints: bootstrapConstraints,
		ModelConstraints:     modelConstraints,
		ModelDefault:         modelDefault,
		StoragePool:          storagePool,
		// The CLI intentionally keeps Juju's single --config entrypoint.
		// StartBootstrap splits bootstrap, controller, and controller-model config,
		// but JIMM merges those maps again when it shells out to juju bootstrap.
		BootstrapConfig: bootstrapConfig,
	}, nil
}

// mergeBootstrapOptions merges parameters from a profile with those explicitly provided for the bootstrap options.
func mergeBootstrapOptions(profile, explicit apiparams.BootstrapOptions) apiparams.BootstrapOptions {
	merged := apiparams.BootstrapOptions{
		BootstrapBase:         profile.BootstrapBase,
		BootstrapConstraints:  mergeMaps(profile.BootstrapConstraints, explicit.BootstrapConstraints),
		ModelConstraints:      mergeMaps(profile.ModelConstraints, explicit.ModelConstraints),
		ModelDefault:          mergeMaps(profile.ModelDefault, explicit.ModelDefault),
		StoragePool:           cloneStoragePool(profile.StoragePool),
		BootstrapConfig:       mergeMaps(profile.BootstrapConfig, explicit.BootstrapConfig),
		ControllerConfig:      mergeMaps(profile.ControllerConfig, explicit.ControllerConfig),
		ControllerModelConfig: mergeMaps(profile.ControllerModelConfig, explicit.ControllerModelConfig),
	}
	if explicit.BootstrapBase != "" {
		merged.BootstrapBase = explicit.BootstrapBase
	}
	if explicit.StoragePool != nil {
		merged.StoragePool = cloneStoragePool(explicit.StoragePool)
	}
	return merged
}

// mergeMaps merges two maps of the same type.
// See maps.Copy where the generic type signatures were lifted from.
func mergeMaps[M1 ~map[K]V, M2 ~map[K]V, K comparable, V any](base M1, override M2) map[K]V {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := maps.Clone(base)
	maps.Copy(merged, override)
	return merged
}

func cloneStoragePool(pool *apiparams.BootstrapStoragePool) *apiparams.BootstrapStoragePool {
	if pool == nil {
		return nil
	}
	return &apiparams.BootstrapStoragePool{
		Name:       pool.Name,
		Type:       pool.Type,
		Attributes: maps.Clone(pool.Attributes),
	}
}

func readStoragePoolFlag(ctxt *cmd.Context, flag *common.ConfigFlag) (*apiparams.BootstrapStoragePool, error) {
	attrs, err := flag.ReadAttrs(ctxt)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage pool values: %w", err)
	}
	if len(attrs) == 0 {
		return nil, nil
	}
	values, err := stringifyMapValues(attrs, "storage-pool")
	if err != nil {
		return nil, err
	}
	pool := &apiparams.BootstrapStoragePool{
		Name: values["name"],
		Type: values["type"],
	}
	delete(values, "name")
	delete(values, "type")
	if pool.Name == "" || pool.Type == "" {
		return nil, fmt.Errorf("storage-pool requires both name and type")
	}
	if len(values) > 0 {
		pool.Attributes = values
	}
	return pool, nil
}

func readStringMapFlag(ctxt *cmd.Context, flag *common.ConfigFlag, flagName string) (map[string]string, error) {
	attrs, err := flag.ReadAttrs(ctxt)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s values: %w", flagName, err)
	}
	return stringifyMapValues(attrs, flagName)
}

func stringifyMapValues(attrs map[string]any, flagName string) (map[string]string, error) {
	if len(attrs) == 0 {
		return nil, nil
	}
	values := make(map[string]string, len(attrs))
	for key, value := range attrs {
		strValue, err := stringifyScalar(value)
		if err != nil {
			return nil, fmt.Errorf("%s value for %q must be a scalar, got %T", flagName, key, value)
		}
		values[key] = strValue
	}
	return values, nil
}

func stringifyScalar(value any) (string, error) {
	if value == nil {
		return "", fmt.Errorf("nil value")
	}
	switch value.(type) {
	case string,
		bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprint(value), nil
	default:
		return "", fmt.Errorf("unsupported type %T", value)
	}
}

func parseConstraintFlag(ctxt *cmd.Context, values []string, flagName string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	// ParseWithAliases requires only spaces and name=value pairs.
	joined := strings.Join(values, " ")
	_, aliases, err := constraints.ParseWithAliases(joined)
	common.WarnConstraintAliases(ctxt, aliases)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", flagName, err)
	}
	parsedValues := splitEscapedFields(joined)
	result := make(map[string]string, len(parsedValues))
	for _, value := range parsedValues {
		keyValue := strings.SplitN(value, "=", 2)
		if len(keyValue) != 2 {
			return nil, fmt.Errorf("failed to parse %s: invalid constraint %q", flagName, value)
		}
		key := keyValue[0]
		if canonical, ok := aliases[key]; ok {
			key = canonical
		}
		result[key] = keyValue[1]
	}
	return result, nil
}

// splitEscapedFields splits a whitespace-delimited constraint string while
// preserving spaces that were escaped as `\ ` inside an individual field.
func splitEscapedFields(value string) []string {
	if value == "" {
		return nil
	}
	normalized := strings.ReplaceAll(value, `\ `, "\x00")
	rawFields := strings.Fields(normalized)
	fields := make([]string, 0, len(rawFields))
	for _, field := range rawFields {
		fields = append(fields, strings.ReplaceAll(field, "\x00", " "))
	}
	return fields
}
