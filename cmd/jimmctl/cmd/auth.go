// Copyright 2024 Canonical.

package cmd

import (
	jujucmd "github.com/juju/cmd/v3"
)

const authDoc = `
The auth command enables user access management.
`

func NewAuthCommand() *jujucmd.SuperCommand {
	cmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name:        "auth",
		UsagePrefix: "jimmctl",
		Doc:         authDoc,
		Purpose:     "Authorisation model management.",
	})
	cmd.Register(NewGroupCommand())
	cmd.Register(NewRelationCommand())
	cmd.Register(NewRoleCommand())

	return cmd
}
