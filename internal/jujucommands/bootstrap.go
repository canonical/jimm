// Copyright 2025 Canonical.

package jujucommands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/jimm/juju"
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

func RunBootstrapCmd(ctx context.Context, u *openfga.User, p BootstrapCmdParams, jm juju.JujuManager) (<-chan outputLine, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	// TODO: Cannot implement yet. JIMM needs to support adding clouds in general such that we can retrieve
	// the users credential. As it stands, because JIMM doesn't support adding clouds, we cannot add a
	// credential.
	//
	// Once the above is done, the flow is:
	// 1. User wants to use cloud X
	// 2. Get cloud (cred is refd to cloud)
	// 3. Find a users credential for said cloud
	// 4. Populate memstore cred
	// 5. Populate temp PersonalClouds or Populate public-clouds.yaml.
	// 	5.1. Juju flow is, add-cloud <public cloud/private cloud>
	//		 If it's personal, clouds.yaml created. If not, it's just added to controller from public-clouds.yaml
	// 5. Call bootstrap?

	return nil, nil
	// return runCmdWithOutputRetriever(memStore, cmdStr)
}
