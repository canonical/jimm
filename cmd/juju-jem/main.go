// Copyright 2015 Canonical Ltd.
package main

import (
	"os"

	"github.com/juju/cmd"

	"github.com/CanonicalLtd/jem/cmd/juju-jem/jemcmd"
)

func main() {
	ctxt := &cmd.Context{
		Dir:    ".",
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}
	os.Exit(cmd.Main(jemcmd.New(), ctxt, os.Args[1:]))
}
