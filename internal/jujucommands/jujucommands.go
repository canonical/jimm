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

type outputLine struct {
	Line string
	Err  error
}

var (
	cmdPrefix = "juju"
)

// runJujuCmd runs a juju command with the given command string and JUJU_DATA directory.
// It returns a channel that will receive output lines from the command's stdout and stderr.
// The command is run in a separate goroutine, and the context can be used to cancel the command.
func runJujuCmd(ctx context.Context, args []string, jujuDataDir string) (<-chan outputLine, error) {
	cmd := exec.CommandContext(ctx, cmdPrefix, args...)
	cmd.Env = append(cmd.Env, "JUJU_DATA="+jujuDataDir)

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

	outputCh := make(chan outputLine, 10) // buffered to avoid blocking

	var wg sync.WaitGroup
	wg.Add(2)

	readLines := func(r *linereader.Reader) {
		defer wg.Done()
		for line := range r.Ch {
			select {
			case outputCh <- outputLine{Line: line}:
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
			outputCh <- outputLine{Err: err}
		}

		close(outputCh)
	}()

	return outputCh, nil
}
