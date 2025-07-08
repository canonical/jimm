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
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/lxd"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// BootstrapCmdParams holds the parameters to bootstrap a controller for JIMM.
type BootstrapCmdParams struct {
	CloudNameAndRegion   string
	ControllerName       string
	AgentVersion         string
	BootstrapTimeout     int
	LoginTokenRefreshURL string
}

func (b BootstrapCmdParams) validate() error {
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

	if b.BootstrapTimeout < 0 {
		return errors.New("bootstrap timeout cannot be less than or equal to 0")
	}

	if b.LoginTokenRefreshURL == "" {
		return errors.New("login-token-refresh-url cannot be empty")
	}

	return nil
}

func (b BootstrapCmdParams) buildBootstrapCmdStr() string {
	var builder strings.Builder
	builder.WriteString("bootstrap")

	// Conditionally add --agent-version if set
	if b.AgentVersion != "" {
		builder.WriteString(fmt.Sprintf(" --agent-version=%s", b.AgentVersion))
	}

	defaultModelName := fmt.Sprintf("%s-%s", b.ControllerName, "default")
	builder.WriteString(fmt.Sprintf(" --add-model=%s", defaultModelName))

	// Conditionally add bootstrap-timeout if set
	if b.BootstrapTimeout > 0 {
		builder.WriteString(fmt.Sprintf(" --config bootstrap-timeout=%d", b.BootstrapTimeout))
	}

	// Always add controller name & cloud at the end
	builder.WriteString(fmt.Sprintf(" %s", b.CloudNameAndRegion))
	builder.WriteString(fmt.Sprintf(" %s", b.ControllerName))
	return builder.String()
}

// RunBootstrapCmd enables the caller to a bootstrap a controller that is ready to be added
// to JIMM. The caller may specify just a credential and empty personal cloud if the target
// cloud is a known public cloud. If it isn't, the personal cloud must be correctly populated.
//
// It returns a output channel which is closed once the command completes. Additionally,
// it returns a closure which cleans up the temporary $JUJU_DATA directory created for the
// lifetime of this command.
func RunBootstrapCmd(
	ctx context.Context,
	p BootstrapCmdParams,
	personalCloud jujucloud.Cloud,
	cred jujucloud.CloudCredential,
	pubKey []byte,
	privKey []byte,
) (<-chan outputLine, func(), error) {
	if err := p.validate(); err != nil {
		return nil, nil, err
	}

	memStore := jujuclient.NewMemStore()

	// Setup JUJU_DATA
	tmpJujuData, err := os.MkdirTemp("", "juju-data-*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp JUJU_DATA: %w", err)
	}

	zapctx.Debug(ctx, "Setting JUJU_DATA path", zap.String("path", tmpJujuData))
	os.Setenv("JUJU_DATA", tmpJujuData)
	// This didn't work... Need to figure a way to not use env var.
	// if err := cmdCtx.Setenv("JUJU_DATA", tmpJujuData); err != nil {
	// 	return nil, nil, err
	// }

	// Setup SSH keys
	// TODO: This isn't working, it's using hosts SSH keys (how do I specify this?)
	// sshDir := filepath.Join(tmpJujuData, "ssh")
	// if err := os.MkdirAll(sshDir, 0700); err != nil {
	// 	return nil, nil, fmt.Errorf("failed to create .ssh directory: %w", err)
	// }

	// if err := os.WriteFile(sshDir+"/juju_id_rsa.pub", []byte(pubKey), 0600); err != nil {
	// 	return nil, nil, fmt.Errorf("writing public key failed: %w", err)
	// }

	// if err = os.WriteFile(sshDir+"/juju_id_rsa", []byte(privKey), 0600); err != nil {
	// 	return nil, nil, fmt.Errorf("writing private key failed: %w", err)
	// }

	// Update public clouds
	// TODO: Make this a command of this package
	outputCh, err := runCmdWithOutputRetriever(memStore, "update-public-clouds --client")
	if err != nil {
		return nil, nil, err
	}

	for line := range outputCh {
		if line.Err != nil {
			return nil, nil, fmt.Errorf("failed to update public clouds: %w", err)
		}
	}

	// Check if we can get the cloud as a public cloud.
	cloudName, regionName := splitCloudNameAndRegion(p.CloudNameAndRegion)
	isAPublicCloud, err := isAValidPublicCloud(cloudName, regionName)
	if err != nil {
		return nil, nil, err
	}
	if !isAPublicCloud {
		// We presume it is a personal cloud
		// TODO: Check if credential should be cloudname or include region
		if err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
			cloudName: personalCloud,
		}); err != nil {
			return nil, nil, fmt.Errorf("failed to write personal cloud: %w", err)
		}
	}

	// TODO: check if cloudName should include region, presuming not right now
	if err := memStore.UpdateCredential(cloudName, cred); err != nil {
		return nil, nil, fmt.Errorf("failed to set credential: %w", err)
	}

	// With the clouds set, credentials updated, we now bootstrap.
	cmdStr := p.buildBootstrapCmdStr()

	cleanupTmpJujuData := func() {
		os.Unsetenv("JUJU_DATA")
		os.RemoveAll(tmpJujuData)
	}

	outputRetriever, err := runCmdWithOutputRetriever(memStore, cmdStr)
	return outputRetriever, cleanupTmpJujuData, err
}

// isAValidPublicCloud checks if the cloud name (and possibly region) is a valid
// public cloud and region. If it is a public cloud without the region specified,
// just the cloud name is checked. If a region is specified, but it isn't valid,
// an error is returned.
//
// TODO: Is there a better way to do this? (Rather than look up metadata and loop through)
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
