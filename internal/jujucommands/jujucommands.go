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
func runCmdWithOutputRetriever(store jujuclient.ClientStore, cmdAndArgs string) (<-chan outputLine, error) {
	// memStore := jujuclient.NewEmbeddedMemStore()
	cmdReader, cmdWriter := io.Pipe()

	cmdCtx, err := cmd.DefaultContext()
	if err != nil {
		return nil, err
	}

	cmdCtx.Stderr = cmdWriter
	cmdCtx.Stdout = cmdWriter

	outputCh := make(chan outputLine)
	// We buffer cmdFinishedCh with capacity 1 to avoid a deadlock here.
	// In runCmd, the writer (cmdWriter) is deferred to close only after runCmd completes.
	// When sending the final error down cmdFinishedCh, if it's unbuffered,
	// the send blocks because no goroutine is receiving yet.
	// This blocks runCmd from completing, so the deferred writer.Close() never runs,
	// meaning no EOF is sent to the reader, which keeps blocking forever.
	// By buffering cmdFinishedCh, the send won't block, allowing runCmd to complete,
	// the writer to close, and the reader to receive EOF and finish reading.
	//
	// There's no need to worry about it blocking, as we'll only ever send one value down
	// once the command completes.
	//
	// We could alternatively manually call cmdWriter.Close() immediately after running the command.
	cmdFinishedCh := make(chan error, 1)

	go func() {
		// Read cmdoutput.
		lr := linereader.New(cmdReader)
		for line := range lr.Ch {
			outputCh <- outputLine{Line: line}
		}

		// Wait for cmd to finish.
		err := <-cmdFinishedCh
		if err != nil {
			outputCh <- outputLine{Err: err}
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
// Note, the cmdAndArgs argument expects a fully-qualified command strings. I.e.,
//
//	"bootstrap lxd a --add-model=\"my-initial-model\""
func runCmd(
	cmdCtx *cmd.Context,
	cmdWriter *io.PipeWriter,
	store jujuclient.ClientStore,
	cmdFinishedCh chan<- error,
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
		true,
	)

	code := cmd.Main(jujuCmd, cmdCtx, strings.Split(cmdAndArgs, " "))

	if code != 0 {
		cmdFinishedCh <- fmt.Errorf("cmd failed with code %d", code)
	} else {
		cmdFinishedCh <- nil
	}
}
