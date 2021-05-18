// Copyright 2021 Canonical Ltd.

package main

import (
	"fmt"
	"os"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

var jimmctlDoc = `
jimmctl enables users to manage JIMM.
`

var log = loggo.GetLogger("jimmctl")

func NewSuperCommand() *jujucmd.SuperCommand {
	jimmcmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name: "jimmctl",
		Doc:  jimmctlDoc,
	})
	jimmcmd.Register(cmd.NewListControllersCommand())
	jimmcmd.Register(cmd.NewModelStatusCommand())
	jimmcmd.Register(cmd.NewRevokeAuditLogAccessCommand())
	jimmcmd.Register(cmd.NewGrantAuditLogAccessCommand())
	return jimmcmd
}

func main() {
	ctx, err := jujucmd.DefaultContext()
	if err != nil {
		fmt.Printf("failed to get command context: %v\n", err)
		os.Exit(2)
	}
	superCmd := NewSuperCommand()
	args := os.Args

	os.Exit(jujucmd.Main(superCmd, ctx, args[1:]))
}
