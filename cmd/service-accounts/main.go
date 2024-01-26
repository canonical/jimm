// Copyright 2021 Canonical Ltd.

package main

import (
	"fmt"
	"os"

	jujucmd "github.com/juju/cmd/v3"
)

var serviceAccountDoc = `
juju service-accounts enables users to manage service accounts.
`

func NewSuperCommand() *jujucmd.SuperCommand {
	serviceAccountCmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name: "service-accounts",
		Doc:  serviceAccountDoc,
	})
	// Register commands here:
	// serviceAccountCmd.Register(cmd.NewCommand())
	return serviceAccountCmd
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
