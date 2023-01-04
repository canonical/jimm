// Copyright 2021 Canonical Ltd.

package jimmtest

import (
	"net/http"
	"os"
	"os/exec"
	"time"

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

	ping := func() error {
		resp, err := http.Get("http://127.0.0.1:8082/healthz")
		if err != nil {
			return errors.E(err, "health ping failed")
		}
		if resp.StatusCode != http.StatusOK {
			return errors.E("health ping failed")
		}
		return nil
	}
	var err error
	for i := 0; i < 10; i++ {
		err = ping()
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return errors.E("failed to start openfga")
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
