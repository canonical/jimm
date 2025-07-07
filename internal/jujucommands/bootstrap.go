// Copyright 2025 Canonical.

package jujucommands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/juju/juju/cloud"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/openfga"
)

type BootstrapCmdParams struct {
	CloudName            string
	ControllerName       string
	AgentVersion         string
	BootstrapTimeout     int
	LoginTokenRefreshURL string
}

func (b BootstrapCmdParams) validate() error {
	if b.CloudName == "" {
		return errors.New("cloud name cannot be empty")
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
	builder.WriteString(fmt.Sprintf(" %s", b.CloudName))
	builder.WriteString(fmt.Sprintf(" %s", b.ControllerName))
	return builder.String()
}

func RunBootstrapCmd(
	ctx context.Context,
	u *openfga.User,
	p BootstrapCmdParams,
	cred cloud.Credential,
) (<-chan outputLine, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	// memStore := jujuclient.NewMemStore()
	// Update public clouds and set JUJU_HOME
	os.Setenv("JUJU_HOME", "./")

	// 1. Create clouds.yaml if cloud isn't a public cloud
	// TODO: Create temp file and set JUJU_HOME and make a todo about how mem store gotta model clouds
	// so we can run multiple bootstraps, cause we currently gotta use JUJU_HOME and clouds.yaml (PersonalCLoud w/e it called)
	// will be location then delete it after

	// 2. Add credential to mem store

	// Add cloud call looks like:
	// func (c *Client) AddCloud(cloud jujucloud.Cloud, force bool) error {
	// args := params.AddCloudArgs{Name: cloud.Name, Cloud: cloudToParams(cloud)}
	// if force {

	// cloud.Cloud{}
	// memStore.UpdateCredential(p.CloudName)

	// Send cloud and creds on facade
	// Call bootstrap with them in memory
	// Add controller to db
	// Add cloud to db (retrieve it from controller)
	// Add credential to db

	return nil, nil
	// return runCmdWithOutputRetriever(memStore, cmdStr)
}
