// Copyright 2021 Canonical Ltd.

package main

import (
	"fmt"
	"os"

	"github.com/canonical/jimm/cmd/jaas/cmd"
	jujucmd "github.com/juju/cmd/v3"
)

var jaasDoc = `
juju jaas enables users to use JAAS commands from within Juju.

JAAS enables enterprise functionality on top of Juju to provide
functionality like OIDC login, control over many controllers,
and fine-grained authorisation.
`

func NewSuperCommand() *jujucmd.SuperCommand {
	serviceAccountCmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name: "jaas",
		Doc:  jaasDoc,
	})
	// Register commands here:
	serviceAccountCmd.Register(cmd.NewAddServiceAccountCommand())
	serviceAccountCmd.Register(cmd.NewListServiceAccountCredentialsCommand())
	serviceAccountCmd.Register(cmd.NewUpdateCredentialsCommand())
	serviceAccountCmd.Register(cmd.NewGrantCommand())
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
