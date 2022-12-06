// Copyright 2021 Canonical Ltd.

package jimmtest

import (
	"os"
	"os/exec"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

var openFGACmd *exec.Cmd

// StartOpenFGA starts and initialises the OpenFGA service.
func StartOpenFGA() error {
	if openFGACmd != nil {
		return errors.E("openFGA already started")
	}
	openFGACmd = exec.Command("openfga", "run", "--http-addr", "0.0.0.0:8082")
	if err := openFGACmd.Start(); err != nil {
		return err
	}
	return nil
}

// StopOpenFGA stops the running OpenFGA server.
func StopOpenFGA() {
	if openFGACmd == nil || openFGACmd.Process == nil {
		return
	}
	if err := openFGACmd.Process.Signal(os.Interrupt); err != nil {
		return
	}
	if err := openFGACmd.Wait(); err != nil {
		return
	}
	openFGACmd = nil
}
