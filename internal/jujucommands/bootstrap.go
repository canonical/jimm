// Copyright 2025 Canonical.

package jujucommands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/lxd"
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
	LoginTokenRefreshURL string

	// Additional args required (like adding credential, cloud, etc.) but JIMM will handle.

	// May be left unset, if set, a personal cloud will be created and used for bootstrap.
	PersonalCloud jujucloud.Cloud
	// The credential to use for the cloud.
	CloudCred jujucloud.Credential

	// UserConfig holds user defined config for bootstrap.
	UserConfig map[string]string
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

	if _, ok := b.UserConfig[loginTokenRefreshURLKey]; ok {
		return fmt.Errorf("%q is a reserved config key and cannot be set in user config", loginTokenRefreshURLKey)
	}

	if b.LoginTokenRefreshURL == "" {
		return errors.New("missing login token refresh URL, this value should be automatically set by JIMM")
	}

	return nil
}

// BuildBootstrapCmdArgs builds the command arguments for the bootstrap command.
func (b BootstrapCmdParams) BuildBootstrapCmdArgs() []string {
	var args []string
	args = append(args, "bootstrap")

	args = append(args, "--config")
	args = append(args, fmt.Sprintf("login-token-refresh-url=%s", b.LoginTokenRefreshURL))

	// Conditionally add --agent-version if set
	if b.AgentVersion != "" {
		args = append(args, fmt.Sprintf("--agent-version=%s", b.AgentVersion))
	}

	for k, v := range b.UserConfig {
		args = append(args, "--config")
		args = append(args, fmt.Sprintf("%s=%s", k, fmt.Sprint(v)))
	}

	// Always add controller name & cloud at the end
	args = append(args, b.CloudNameAndRegion, b.ControllerName)

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
	// TODO: Move this command to it's own file.
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

	// Check if we can get the cloud as a public cloud.
	cloudName, regionName := splitCloudNameAndRegion(p.CloudNameAndRegion)
	isAPublicCloud, err := isAValidPublicCloud(cloudName, regionName)
	if err != nil {
		return nil, nil, nil, err
	}
	if !isAPublicCloud {
		// We presume it is a personal cloud
		if err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
			cloudName: p.PersonalCloud,
		}); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to write personal cloud: %w", err)
		}
	}

	// Create a cloudCredential at the last possible moment from the provided credential.
	// A cloudCredential holds a map of credentials for the cloud with optional defaults.
	// We only accept a single credential for bootstrapping.
	cloudCred := jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			p.CloudCred.Label: p.CloudCred,
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
