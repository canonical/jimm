// Copyright 2025 Canonical.

// Package jujucommands provides functions run juju cmds from a JIMM instance.
// Each command function is run with its own isolated in-mem store.
package jujucommands

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/jujuclient"
	"github.com/mitchellh/go-linereader"
)

var (
	whitelist = []string{"help", "bootstrap"}
)

type outputLine struct {
	Line string
	Err  error
}

// runCmdWithOutputRetriever runs the command given by cmdAndArgs using the provided jujuclient.ClientStore.
// It returns a channel that streams OutputLine values for each line of command output and any final error.
//
// Upon receiving an error in an OutputLine, the line previously streamed is guaranteed to be the error line written by
// the command.
// I.e., if you run: "help -b"
// You will get:
//
//	Output line: ERROR option provided but not defined: -b // The error line from juju (Which is a none-erroneous OutputLine)
//	Command finished with error: cmd failed with code 2 // The command error code line (Contains an error in the OutputLine)
//
// Consumers are expected to simply read from the OutputLine channel and check for an error like so:
//
//	for out := range outputCh {
//		if out.Err != nil {
//			fmt.Println("Command finished with error:", out.Err)
//			break
//		}
//		fmt.Println("Output line:", out.Line)
//	}
//
// The OutputLine channel is closed once all output from the command has finished an the error has been captured.
//
// Lines returned are returned with no newlines.
func runCmdWithOutputRetriever(store jujuclient.ClientStore, cmdAndArgs string) (<-chan outputLine, error) {
	cmdReader, cmdWriter := io.Pipe()

	cmdCtx, err := cmd.DefaultContext()
	if err != nil {
		return nil, err
	}

	cmdCtx.Stderr = cmdWriter
	cmdCtx.Stdout = cmdWriter

	outputCh := make(chan outputLine)
	// We buffer cmdFinishedCh (capacity 1) to avoid deadlock.
	// runCmd defers cmdWriter.Close(), which only runs after runCmd finishes.
	// If cmdFinishedCh were unbuffered, sending the final error would block
	// (since no goroutine is receiving yet), which prevents runCmd from finishing.
	// This means cmdWriter.Close() never runs, so the reader never gets EOF and hangs.
	// With a buffered channel, the send doesn't block, allowing runCmd to finish,
	// the writer to close, and the reader to eventually see EOF and exit.
	//
	// This is safe because we only send once, after the command finishes.
	// Alternatively, we could manually close cmdWriter right after the command.
	cmdFinishedCh := make(chan int, 1)

	go func() {
		// Read cmdoutput.
		lr := linereader.New(cmdReader)
		for line := range lr.Ch {
			outputCh <- outputLine{Line: line}
		}

		// Wait for cmd to finish.
		code := <-cmdFinishedCh
		if code != 0 {
			outputCh <- outputLine{Err: fmt.Errorf("cmd failed with code: %d", code)}
		}

		close(outputCh)
	}()
	go runCmd(cmdCtx, cmdWriter, store, cmdFinishedCh, cmdAndArgs)

	return outputCh, nil
}

// runCmd executes a Juju command using the provided context, client store, and command arguments.
// It writes command output to the given io.PipeWriter and signals completion or error via cmdFinishedCh.
// The function splits cmdAndArgs into arguments, constructs a Juju command, and runs it.
// On non-zero exit code, it sends an error to cmdFinishedCh; otherwise, it signals successful completion.
//
// Note, the cmdAndArgs argument expects a fully-qualified command string. I.e.,
//
//	"bootstrap lxd a --add-model=\"my-initial-model\""
func runCmd(
	cmdCtx *cmd.Context,
	cmdWriter *io.PipeWriter,
	store jujuclient.ClientStore,
	cmdFinishedCh chan<- int,
	cmdAndArgs string,
) {
	defer cmdWriter.Close()

	jujuCmd := commands.NewJujuCommandWithStore(
		cmdCtx,
		store,
		nil,       // quiet?
		"",        // unknown param
		"",        // no help hint
		whitelist, // whitelist
		// We set embedded false because some commands can targets either a controller or client.
		// See: https://github.com/juju/juju/blob/1abcb8c5f1f187fd1443e12d6324a47780b43740/cmd/modelcmd/controller.go#L451
		// And whilst we are "technically" embedding the CLI, without setting this to false,
		// You must run without --client/--controller and it defaults to updating the client
		// in update-public-clouds. So, we set it false to allow us to explicitly set
		// "--client" and not have it ignored.
		false,
	)

	code := cmd.Main(jujuCmd, cmdCtx, strings.Split(cmdAndArgs, " "))

	cmdFinishedCh <- code
}
