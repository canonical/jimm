// Copyright 2025 Canonical.

package jujucommands

import "context"

// Runner defines a juju command runner.
type Runner interface {
	JujuDataDir() string
	RunJujuCmd(ctx context.Context, args []string) (<-chan OutputLine, error)
}
