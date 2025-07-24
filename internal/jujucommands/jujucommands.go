// Copyright 2025 Canonical.

// Package jujucommands provides functions run juju cmds from a JIMM instance.
// Each command function is run with its own isolated in-mem store.
package jujucommands

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/mitchellh/go-linereader"
)

var (
	cmdPrefix = "juju"
)

// OutputLine represents a line of output from a juju command.
type OutputLine struct {
	Line string
	Err  error
}

// CommandRunner is a struct that runs juju commands and JUJU_DATA directory.
type CommandRunner struct {
	prefix      string
	jujuDataDir string
}

// NewCommandRunner creates a new CommandRunner with the specified command prefix.
func NewCommandRunner(dataDir string) *CommandRunner {
	return &CommandRunner{
		prefix:      cmdPrefix,
		jujuDataDir: dataDir,
	}
}

// runJujuCmd runs a juju command with the given command string and JUJU_DATA directory.
// It returns a channel that will receive output lines from the command's stdout and stderr.
// The command is run in a separate goroutine, and the context can be used to cancel the command.
func (b *CommandRunner) RunJujuCmd(ctx context.Context, args []string) (<-chan OutputLine, error) {
	cmd := exec.CommandContext(ctx, b.prefix, args...)
	cmd.Env = append(cmd.Env, "JUJU_DATA="+b.jujuDataDir)

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout: %w", err)
	}

	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	outputCh := make(chan OutputLine, 10) // buffered to avoid blocking

	var wg sync.WaitGroup
	wg.Add(2)

	readLines := func(r *linereader.Reader) {
		defer wg.Done()
		for line := range r.Ch {
			select {
			case outputCh <- OutputLine{Line: line}:
			case <-ctx.Done():
				return
			}
		}
	}

	go readLines(linereader.New(stdOut))
	go readLines(linereader.New(stdErr))

	go func() {
		// We wait for readers to finished in the case the command exits but
		// the readers are still processing output. Small chance this could happen,
		// but this protects us.
		wg.Wait()

		if err := cmd.Wait(); err != nil {
			outputCh <- OutputLine{Err: err}
		}

		close(outputCh)
	}()

	return outputCh, nil
}
