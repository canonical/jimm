// Copyright 2021 Canonical Ltd.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/canonical/jimm/cmd/jaas/cmd"
	jujucmd "github.com/juju/cmd/v3"
)

var jaasDoc = `
jaas enables users to use JAAS commands from within the Juju CLI.

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
	serviceAccountCmd.Register(cmd.NewAddServiceAccountCredentialCommand())
	return serviceAccountCmd
}

const (
	jujuPrefix  = "juju-"
	jaasCommand = "juju-jaas"
)

func main() {
	ctx, err := jujucmd.DefaultContext()
	if err != nil {
		fmt.Printf("failed to get command context: %v\n", err)
		os.Exit(2)
	}
	superCmd := NewSuperCommand()
	var args []string
	// The following if condition handles cases where the juju binary calls jaas as a plugin.
	// Symlinks of the form juju-<command> are created to make all jaas commands appear as top
	// level commands to the Juju CLI and then we strip the juju- prefix to obtain the desired function.
	if strings.HasPrefix(os.Args[0], jujuPrefix) && os.Args[0] != jaasCommand {
		args = make([]string, len(os.Args))
		copy(args[1:], os.Args[1:])
		args[0] = strings.TrimPrefix(os.Args[0], "juju-")
	} else {
		args = os.Args[1:]
	}
	os.Exit(jujucmd.Main(superCmd, ctx, args))
}
