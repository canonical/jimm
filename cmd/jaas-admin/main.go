// Copyright 2015 Canonical Ltd.
package main

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/juju/juju"

	"github.com/canonical/jimm/cmd/jaas-admin/admincmd"
)

func main() {
	if err := juju.InitJujuXDGDataHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	ctxt := &cmd.Context{
		Dir:    ".",
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}
	os.Exit(cmd.Main(admincmd.New(), ctxt, os.Args[1:]))
}
