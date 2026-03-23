// Copyright 2025 Canonical.

package jujucommands

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/version/v2"
)

const (
	//nolint:gosec // Thinks hardcoded credentials.
	loginTokenRefreshURLKey = "login-token-refresh-url"
)

// BootstrapCmdParams holds the parameters to bootstrap a controller for JIMM.
type BootstrapCmdParams struct {
	// Arguments to be turned into an actual command str.
	CloudNameAndRegion   string
	ControllerName       string
	AgentVersion         string
	DefaultLoginTokenURL string

	// Additional args required (like adding credential, cloud, etc.) but JIMM will handle.

	// Cloud contains the details for the cloud.
	// Only expected to be set if the cloud is not a public cloud (i.e. not AWS, Azure, etc).
	Cloud jujucloud.Cloud
	// The credential to use for the cloud.
	CloudCred jujucloud.Credential

	// BootstrapOptions holds the supported bootstrap settings.
	BootstrapOptions BootstrapOptions
}

// BootstrapOptions holds the supported bootstrap settings rendered into Juju CLI flags.
type BootstrapOptions struct {
	BootstrapBase         string
	BootstrapConstraints  map[string]string
	ModelConstraints      map[string]string
	ModelDefault          map[string]string
	StoragePool           *StoragePool
	BootstrapConfig       map[string]string
	ControllerConfig      map[string]string
	ControllerModelConfig map[string]string
}

// StoragePool describes an initial controller-model storage pool.
type StoragePool struct {
	Name       string
	Type       string
	Attributes map[string]string
}

// Validate validates the BootstrapCmdParams.
func (b BootstrapCmdParams) Validate() error {
	if b.CloudNameAndRegion == "" {
		return errors.New("cloud [and region] name cannot be empty")
	}

	if b.ControllerName == "" {
		return errors.New("controller name name cannot be empty")
	}

	if b.AgentVersion != "" {
		if _, err := version.ParseBinary(b.AgentVersion); err != nil {
			if _, err := version.Parse(b.AgentVersion); err != nil {
				return err
			}
		}
	}

	if b.DefaultLoginTokenURL == "" {
		return errors.New("missing login token refresh URL, this value should be automatically set by JIMM")
	}

	if pool := b.BootstrapOptions.StoragePool; pool != nil && (pool.Name == "" || pool.Type == "") {
		return errors.New("storage pool requires both name and type")
	}

	if arch, ok := b.BootstrapOptions.BootstrapConstraints["arch"]; ok && arch != runtime.GOARCH {
		return fmt.Errorf("bootstrap constraint arch must be %q", runtime.GOARCH)
	}

	return nil
}

// BuildBootstrapCmdArgs builds the command arguments for the bootstrap command.
func (b BootstrapCmdParams) BuildBootstrapCmdArgs() []string {
	var args []string
	args = append(args, "bootstrap")

	// Conditionally add --agent-version if set
	if b.AgentVersion != "" {
		args = append(args, fmt.Sprintf("--agent-version=%s", b.AgentVersion))
	}

	if b.BootstrapOptions.BootstrapBase != "" {
		args = append(args, fmt.Sprintf("--bootstrap-base=%s", b.BootstrapOptions.BootstrapBase))
	}

	args = appendKeyValueFlags(args, "bootstrap-constraints", withBootstrapArch(b.BootstrapOptions.BootstrapConstraints))
	args = appendKeyValueFlags(args, "constraints", b.BootstrapOptions.ModelConstraints)
	args = appendKeyValueFlags(args, "model-default", b.BootstrapOptions.ModelDefault)
	args = appendStoragePoolFlags(args, b.BootstrapOptions.StoragePool)
	args = appendConfigFlags(args, mergedBootstrapConfig(b.DefaultLoginTokenURL, b.BootstrapOptions))

	// Always add controller name & cloud at the end
	args = append(args, b.CloudNameAndRegion, b.ControllerName)

	return args
}

func withBootstrapArch(constraints map[string]string) map[string]string {
	merged := maps.Clone(constraints)
	if merged == nil {
		merged = make(map[string]string, 1)
	}
	// Add architecture constraint to ensure juju bootstraps with the correct arch.
	// Without this, the controller application that is deployed can be deployed with the wrong architecture.
	merged["arch"] = runtime.GOARCH
	return merged
}

func mergedBootstrapConfig(defaultLoginTokenURL string, options BootstrapOptions) map[string]string {
	merged := make(map[string]string)
	for _, configMap := range []map[string]string{options.ControllerConfig, options.ControllerModelConfig, options.BootstrapConfig} {
		for key, value := range configMap {
			merged[key] = value
		}
	}
	if _, ok := merged[loginTokenRefreshURLKey]; !ok {
		merged[loginTokenRefreshURLKey] = defaultLoginTokenURL
	}
	return merged
}

func appendConfigFlags(args []string, config map[string]string) []string {
	if len(config) == 0 {
		return args
	}
	if value, ok := config[loginTokenRefreshURLKey]; ok {
		args = append(args, "--config", fmt.Sprintf("%s=%s", loginTokenRefreshURLKey, value))
	}
	remaining := maps.Clone(config)
	delete(remaining, loginTokenRefreshURLKey)
	return appendKeyValueFlags(args, "config", remaining)
}

func appendKeyValueFlags(args []string, flagName string, values map[string]string) []string {
	if len(values) == 0 {
		return args
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, fmt.Sprintf("--%s", flagName), fmt.Sprintf("%s=%s", key, values[key]))
	}
	return args
}

func appendStoragePoolFlags(args []string, pool *StoragePool) []string {
	if pool == nil {
		return args
	}
	args = append(args, "--storage-pool", fmt.Sprintf("name=%s", pool.Name))
	args = append(args, "--storage-pool", fmt.Sprintf("type=%s", pool.Type))
	if len(pool.Attributes) == 0 {
		return args
	}
	keys := make([]string, 0, len(pool.Attributes))
	for key := range pool.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--storage-pool", fmt.Sprintf("%s=%s", key, pool.Attributes[key]))
	}
	return args
}

type bootstrapCmd struct {
	runner Runner
}

// NewBootstrapCmd creates a new BootstrapCmd with the specified command runner.
func NewBootstrapCmd(runner Runner) *bootstrapCmd {
	return &bootstrapCmd{
		runner: runner,
	}
}

// Run enables the caller to a bootstrap a controller that is ready to be added
// to JIMM. The caller may specify just a credential and empty personal cloud if the target
// cloud is a known public cloud. If it isn't, the personal cloud must be correctly populated.
//
// It returns a output channel which is closed once the command completes. Additionally,
// it returns a closure which cleans up the temporary $JUJU_DATA directory created for the
// lifetime of this command.
//
// User SSH keys will be added after the bootstrap utilising JIMM's JWT authentication.
func (c *bootstrapCmd) Run(ctx context.Context, p BootstrapCmdParams) (<-chan OutputLine, jujuclient.ClientStore, func(), error) {
	if err := p.Validate(); err != nil {
		return nil, nil, nil, err
	}

	dataDir := c.runner.JujuDataDir()
	osenv.SetJujuXDGDataHome(dataDir)

	// Update public clouds
	outputCh, err := c.runner.RunJujuCmd(ctx, []string{"update-public-clouds", "--client"})
	if err != nil {
		return nil, nil, nil, err
	}

	for line := range outputCh {
		if line.Err != nil {
			return nil, nil, nil, fmt.Errorf("failed to update public clouds: %w", line.Err)
		}
	}

	store := jujuclient.NewFileClientStore()

	// If a cloud is not known to Juju i.e. clouds besides AWS, Azure, GCP, etc.,
	// then we need to create a cloud entry on disk with information on how
	// to reach the cloud, its regions, etc.
	cloudName, regionName := splitCloudNameAndRegion(p.CloudNameAndRegion)
	isAPublicCloud, err := isAValidPublicCloud(cloudName, regionName)
	if err != nil {
		return nil, nil, nil, err
	}
	if !isAPublicCloud {
		if err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
			cloudName: p.Cloud,
		}); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to write personal cloud: %w", err)
		}
	}

	// Create a cloudCredential at the last possible moment from the provided credential.
	// A cloudCredential holds a map of credentials for the cloud with optional defaults.
	// We only accept a single credential for bootstrapping.
	cloudCred := jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			cloudName: p.CloudCred,
		},
	}

	if err := store.UpdateCredential(cloudName, cloudCred); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to set credential: %w", err)
	}

	// With the clouds set, credentials updated, we now bootstrap.
	args := p.BuildBootstrapCmdArgs()

	cleanupTmpJujuData := func() {
		os.RemoveAll(dataDir)
	}

	outputRetriever, err := c.runner.RunJujuCmd(ctx, args)
	if err != nil {
		return nil, nil, cleanupTmpJujuData, fmt.Errorf("failed to run bootstrap command: %w", err)
	}
	return outputRetriever, store, cleanupTmpJujuData, nil
}

// isAValidPublicCloud checks if the cloud name (and possibly region) is a valid
// public cloud and region. If it is a public cloud without the region specified,
// just the cloud name is checked. If a region is specified, but it isn't valid,
// an error is returned.
func isAValidPublicCloud(cloudName, regionName string) (bool, error) {
	var isAPublicCloud bool

	pubClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return false, fmt.Errorf("failed to get public cloud metadata: %w", err)
	}

	for pubCloudName, cloud := range pubClouds {
		if cloudName == pubCloudName {
			isAPublicCloud = true
			if regionName != "" {
				exists := slices.ContainsFunc(cloud.Regions, func(r jujucloud.Region) bool {
					return regionName == r.Name
				})
				if !exists {
					return false, fmt.Errorf("invalid public cloud region for cloud %s with region %s", cloudName, regionName)
				}
			}
		}
	}

	return isAPublicCloud, nil
}

func splitCloudNameAndRegion(cloudNameAndRegion string) (cloudName string, regionName string) {
	if i := strings.IndexRune(cloudNameAndRegion, '/'); i > 0 {
		cloudName, regionName = cloudNameAndRegion[:i], cloudNameAndRegion[i+1:]
	} else {
		cloudName = cloudNameAndRegion
	}

	return
}
