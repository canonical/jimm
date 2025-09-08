// Copyright 2025 Canonical.

package jujucommands

import (
	"context"
	"errors"
	"fmt"

	"github.com/juju/juju/juju/osenv"
	_ "github.com/juju/juju/provider/lxd"
)

// DestroyControllerCmdParams holds the parameters to tear-down a controller.
type DestroyControllerCmdParams struct {
	ControllerName string
}

// Validate validates the BootstrapCmdParams.
func (b DestroyControllerCmdParams) Validate() error {
	if b.ControllerName == "" {
		return errors.New("controller name cannot be empty")
	}

	return nil
}

// BuildCmdArgs builds the arguments for the command.
func (b DestroyControllerCmdParams) BuildCmdArgs() []string {
	var args []string
	args = append(args, "destroy-controller")
	args = append(args, b.ControllerName)
	args = append(args, "--no-prompt")

	return args
}

type destroyControllerCmd struct {
	runner Runner
}

// NewDestroyControllerCmd creates a new destroyControllerCmd with the specified command runner.
func NewDestroyControllerCmd(runner Runner) *destroyControllerCmd {
	return &destroyControllerCmd{
		runner: runner,
	}
}

// Run runs the destroy-controller command with the given parameters.
func (c *destroyControllerCmd) Run(ctx context.Context, p DestroyControllerCmdParams) (<-chan OutputLine, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	dataDir := c.runner.JujuDataDir()
	osenv.SetJujuXDGDataHome(dataDir)

	args := p.BuildCmdArgs()

	outputRetriever, err := c.runner.RunJujuCmd(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("failed to run bootstrap command: %w", err)
	}
	return outputRetriever, nil
}
