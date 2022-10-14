// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	jujutesting "github.com/juju/juju/testing"
)

var vaultNotAvailable bool

func TestPackage(t *testing.T) {
	cmd := exec.Command("vault", "server", "-dev", "-dev-root-token-id=test-token")
	err := cmd.Start()
	if err != nil {
		vaultNotAvailable = true
		fmt.Fprintf(os.Stderr, "cannot start vault: %s", err)
	} else {
		defer func() {
			cmd.Process.Signal(os.Interrupt)
			cmd.Wait()
		}()
	}
	jujutesting.MgoTestPackage(t)
}
